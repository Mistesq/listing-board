# Test Action

1. Read current-feature.md to understand what was implemented
2. Identify the business logic and utility functions added/modified for this feature (services, server actions, validation, helpers)
3. Check if tests already exist for these functions
4. For functions without tests that have testable logic, write unit tests:
   - Follow the Testing section in context/ai-interaction.md
   - Focus on logic, not components
   - Test happy path and error cases
   - Do not write tests just to write them. Use your best judgement
5. Run the test command from CLAUDE.md to verify all tests pass
6. Report test coverage for the new feature code
