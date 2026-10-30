import { FormEvent, useEffect, useRef, useState } from "react";

type Step = { label: string; state: string };

type Job = {
  id: string;
  title: string;
  address: string;
  status: string;
  error: string;
  runId: string;
  filename: string;
  checklist: Step[];
};

type Account = { id: string; email: string };

class ApiError extends Error {
  readonly status: number;

  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

const pollEvery = 1000;
const listPollEvery = 2000;
const maxBrochureBytes = 1048576;

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, { credentials: "include", ...init });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) {
    const message = data && typeof data.error === "string" ? data.error : res.statusText;
    throw new ApiError(message, res.status);
  }
  return data as T;
}

export function App() {
  const [me, setMe] = useState<Account | null>(null);
  const [ready, setReady] = useState(false);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [selected, setSelected] = useState<Job | null>(null);
  const [error, setError] = useState("");
  const [brochureError, setBrochureError] = useState<{ id: string; message: string } | null>(null);

  useEffect(() => {
    api<Account>("/api/me")
      .then(setMe)
      .catch(() => setMe(null))
      .finally(() => setReady(true));
  }, []);

  useEffect(() => {
    if (!me) {
      setJobs([]);
      return;
    }
    api<Job[]>("/api/jobs")
      .then(setJobs)
      .catch((err: Error) => setError(err.message));
  }, [me]);

  // Follow the open sheet while the worker runs it. Each request is scheduled
  // after the previous response, and a response for a sheet that is no longer
  // open is dropped.
  const openId = selected?.id;
  const running = selected?.status === "running";
  useEffect(() => {
    if (!openId || !running) {
      return;
    }
    let active = true;
    let timer = 0;
    let pollError = "";
    function poll() {
      api<Job>("/api/jobs/" + openId)
        .then((job) => {
          if (!active) {
            return;
          }
          if (pollError) {
            const message = pollError;
            pollError = "";
            setError((current) => (current === message ? "" : current));
          }
          update(job);
          if (job.status === "running") {
            timer = window.setTimeout(poll, pollEvery);
          }
        })
        .catch((err: Error) => {
          if (!active) {
            return;
          }
          pollError = err.message;
          setError(err.message);
          const gone = err instanceof ApiError && (err.status === 401 || err.status === 404);
          if (!gone) {
            timer = window.setTimeout(poll, pollEvery);
          }
        });
    }
    timer = window.setTimeout(poll, pollEvery);
    return () => {
      active = false;
      window.clearTimeout(timer);
      if (pollError) {
        const message = pollError;
        setError((current) => (current === message ? "" : current));
      }
    };
  }, [openId, running]);

  // Refresh the list while a sheet other than the open one is running.
  // Entries are only updated, never added or removed, so a sheet created
  // meanwhile stays; the open sheet is left to its own poll above.
  const openRef = useRef(openId);
  openRef.current = openId;
  const listRunning = jobs.some((job) => job.status === "running" && job.id !== openId);
  useEffect(() => {
    if (!listRunning) {
      return;
    }
    let active = true;
    let timer = 0;
    function poll() {
      api<Job[]>("/api/jobs")
        .then((fresh) => {
          if (!active) {
            return;
          }
          const byId = new Map(fresh.map((job) => [job.id, job]));
          setJobs((current) =>
            current.map((item) => (item.id === openRef.current ? item : (byId.get(item.id) ?? item))),
          );
          timer = window.setTimeout(poll, listPollEvery);
        })
        .catch((err: Error) => {
          if (active && !(err instanceof ApiError && err.status === 401)) {
            timer = window.setTimeout(poll, listPollEvery);
          }
        });
    }
    timer = window.setTimeout(poll, listPollEvery);
    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, [listRunning]);

  function update(job: Job) {
    setSelected((current) => (current?.id === job.id ? job : current));
    setJobs((current) => current.map((item) => (item.id === job.id ? job : item)));
  }

  if (!ready) {
    return <p className="muted">Loading…</p>;
  }
  if (!me) {
    return <Auth onIn={setMe} />;
  }

  return (
    <main>
      <header>
        <div>
          <p className="eyebrow">Real estate</p>
          <h1>Listing board</h1>
        </div>
        <button
          className="ghost"
          onClick={() => {
            api("/api/logout", { method: "POST" }).finally(() => {
              setMe(null);
              setSelected(null);
            });
          }}
        >
          Log out
        </button>
      </header>
      {error ? <p className="error">{error}</p> : null}
      <Create
        onCreated={(job, uploadError) => {
          setJobs((current) => [job, ...current]);
          setSelected(job);
          setError("");
          setBrochureError(
            uploadError
              ? { id: job.id, message: "Sheet created, but the brochure was not attached: " + uploadError }
              : null,
          );
        }}
        onError={setError}
      />
      <section className="layout">
        <ul className="list">
          {jobs.length === 0 ? <li className="muted">No sheets yet.</li> : null}
          {jobs.map((job) => (
            <li key={job.id}>
              <button className={selected?.id === job.id ? "row on" : "row"} onClick={() => setSelected(job)}>
                <span>
                  <strong>{job.title}</strong>
                  <span className="muted">{job.address}</span>
                </span>
                <span className={"status " + job.status}>{job.status}</span>
              </button>
            </li>
          ))}
        </ul>
        {selected ? (
          <Sheet
            key={selected.id}
            job={selected}
            brochureError={brochureError?.id === selected.id ? brochureError.message : ""}
            onChange={update}
            onError={setError}
            onBrochureError={(id, message) =>
              setBrochureError((current) => (message ? { id, message } : current?.id === id ? null : current))
            }
          />
        ) : (
          <p className="muted">Select a sheet.</p>
        )}
      </section>
    </main>
  );
}

function Auth({ onIn }: { onIn: (account: Account) => void }) {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError("");
    try {
      const account = await api<Account>("/api/" + mode, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password }),
      });
      onIn(account);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not sign in");
    }
  }

  return (
    <main className="narrow">
      <p className="eyebrow">Real estate</p>
      <h1>Listing board</h1>
      <form onSubmit={submit}>
        <label>
          Email
          <input value={email} onChange={(event) => setEmail(event.target.value)} type="email" required />
        </label>
        <label>
          Password
          <input value={password} onChange={(event) => setPassword(event.target.value)} type="password" required />
        </label>
        {error ? <p className="error">{error}</p> : null}
        <button type="submit">{mode === "login" ? "Sign in" : "Create account"}</button>
      </form>
      <button className="ghost" onClick={() => setMode(mode === "login" ? "register" : "login")}>
        {mode === "login" ? "Need an account?" : "Already have an account?"}
      </button>
    </main>
  );
}

function Create({
  onCreated,
  onError,
}: {
  onCreated: (job: Job, uploadError?: string) => void;
  onError: (message: string) => void;
}) {
  const [title, setTitle] = useState("");
  const [address, setAddress] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const form = event.currentTarget as HTMLFormElement;
    if (submitting) {
      return;
    }
    if (!file) {
      onError("Choose a brochure file");
      return;
    }
    // Size is checked here so an obviously bad file creates no sheet; the
    // type is still decided by the server from the bytes.
    if (file.size === 0) {
      onError("Brochure must be a PDF or a text file");
      return;
    }
    if (file.size > maxBrochureBytes) {
      onError("Brochure is too large");
      return;
    }
    setSubmitting(true);
    try {
      let job: Job;
      try {
        job = await api<Job>("/api/jobs", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ title, address }),
        });
      } catch (err) {
        onError(err instanceof Error ? err.message : "Could not create the sheet");
        return;
      }
      // The sheet exists from here on, so the form is cleared and the sheet is
      // listed even when the upload fails.
      setTitle("");
      setAddress("");
      setFile(null);
      form.reset();
      try {
        onCreated(await uploadBrochure(job.id, file));
      } catch (err) {
        onCreated(job, err instanceof Error ? err.message : "Could not store the brochure");
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form className="create" onSubmit={submit}>
      <label>
        Title
        <input value={title} onChange={(event) => setTitle(event.target.value)} required />
      </label>
      <label>
        Address
        <input value={address} onChange={(event) => setAddress(event.target.value)} required />
      </label>
      <label>
        Brochure
        <input
          type="file"
          accept=".pdf,.txt,application/pdf,text/plain"
          onChange={(event) => setFile(event.target.files?.[0] ?? null)}
          required
        />
      </label>
      <button type="submit" disabled={submitting}>
        Create sheet
      </button>
    </form>
  );
}

function uploadBrochure(id: string, file: File): Promise<Job> {
  const body = new FormData();
  body.append("file", file);
  return api<Job>("/api/jobs/" + id + "/file", { method: "POST", body });
}

function Sheet({
  job,
  brochureError,
  onChange,
  onError,
  onBrochureError,
}: {
  job: Job;
  brochureError: string;
  onChange: (job: Job) => void;
  onError: (message: string) => void;
  onBrochureError: (id: string, message: string) => void;
}) {
  const [uploading, setUploading] = useState(false);

  async function start() {
    onError("");
    try {
      const next = await api<Job>("/api/jobs/" + job.id + "/start", { method: "POST" });
      onChange(next);
    } catch (err) {
      onError(err instanceof Error ? err.message : "Could not start the sheet");
    }
  }

  async function attach(file: File) {
    onError("");
    setUploading(true);
    try {
      onChange(await uploadBrochure(job.id, file));
      onBrochureError(job.id, "");
    } catch (err) {
      onBrochureError(job.id, err instanceof Error ? err.message : "Could not store the brochure");
    } finally {
      setUploading(false);
    }
  }

  return (
    <article className="sheet">
      <h2>{job.title}</h2>
      <p>{job.address}</p>
      <p className={"status " + job.status}>{job.status}</p>
      {job.filename ? <p className="muted">{job.filename}</p> : null}
      {job.error ? <p className="error">{job.error}</p> : null}
      <ol>
        {job.checklist.map((step) => (
          <li key={step.label} className={step.state}>
            {step.label}
          </li>
        ))}
      </ol>
      {job.status === "draft" && !job.filename ? (
        <label>
          Brochure
          <input
            type="file"
            accept=".pdf,.txt,application/pdf,text/plain"
            disabled={uploading}
            onChange={(event) => {
              const file = event.target.files?.[0];
              event.target.value = "";
              if (file) {
                attach(file);
              }
            }}
          />
        </label>
      ) : null}
      {brochureError ? <p className="error">{brochureError}</p> : null}
      {job.status === "draft" && job.filename ? <button onClick={start}>Start</button> : null}
    </article>
  );
}
