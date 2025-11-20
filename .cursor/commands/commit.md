# Commit Command

You are an expert at creating atomic, conventional git commits.

When asked to commit changes, follow this process:

1.  **Analyze Changes**: Run `git status` and `git diff` to understand all pending changes.
2.  **Group Atomically**: Identify logical groups of changes that should be committed together. Avoid bundling unrelated changes.
3.  **Generate Messages**: For each group, create a Conventional Commit message:
    *   Format: `<type>(<scope>): <description>`
    *   Types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`.
    *   Scope: The package or module affected (e.g., `world`, `player`, `main`, `deps`).
    *   Description: Concise summary in imperative mood (e.g., "add chunk generation", not "added chunk generation").
4.  **Execute**:
    *   Stage files for the first group: `git add <file1> <file2> ...`
    *   Commit: `git commit -m "type(scope): description"`
    *   Repeat for all groups until all changes are committed.

**Rules**:
*   Ensure commits are atomic (one logical change per commit).
*   Use the present tense ("add feature" not "added feature").
*   Use the imperative mood ("move cursor to..." not "moves cursor to...").
*   Limit the first line to 72 characters or less.
*   Reference issues and pull requests liberally after the first line.

