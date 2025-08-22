/**
 * @type {import('@commitlint/types').UserConfig}
 */
module.exports = {
  // Start with the recommended conventional-changelog configuration.
  extends: ['@commitlint/config-conventional'],

  // Add our own stricter and more specific rules.
  rules: {
    //
    // TYPE RULES
    //
    // Define the list of allowed commit types. This is the most important rule.
    'type-enum': [
      2, // Level: Error. A commit will be rejected if its type is not in this list.
      'always',
      [
        // ---- Main Types (will appear in the changelog) ----
        'feat',     // A new feature.
        'fix',      // A bug fix.
        'perf',     // A code change that improves performance.

        // ---- Other Types (typically won't appear in the changelog) ----
        'refactor', // A code change that neither fixes a bug nor adds a feature.
        'style',    // Changes that do not affect the meaning of the code (white-space, formatting, etc).
        'docs',     // Documentation only changes.
        'test',     // Adding missing tests or correcting existing tests.
        'build',    // Changes that affect the build system or external dependencies (e.g., GoReleaser, npm).
        'ci',       // Changes to our CI configuration files and scripts (e.g., GitHub Actions).
        'chore',    // Other changes that don't modify src or test files (e.g., updating .gitignore).
        'revert',   // Reverts a previous commit.
      ],
    ],
    // The type must always be in lower-case.
    'type-case': [2, 'always', 'lower-case'],
    // The type must not be empty.
    'type-empty': [2, 'never'],

    //
    // SCOPE RULES
    //
    // A scope provides additional contextual information. It's optional but recommended.
    // This rule provides a list of suggested scopes for this project.
    'scope-enum': [
      1, // Level: Warning. A commit will not be rejected, but a warning will be shown.
      'always',
      [
        'tui',      // Changes related to the Terminal UI (Bubble Tea components, layout).
        'k8s',      // Changes related to Kubernetes client interaction.
        'cli',      // Changes related to the command-line interface (Cobra flags, args).
        'ci',       // Changes related to CI/CD workflows.
        'docs',     // Changes related to documentation or the README.
        'release',  // Commits related to the release process.
        'deps',     // Dependency updates.
      ]
    ],
    // The scope must always be in lower-case.
    'scope-case': [2, 'always', 'lower-case'],

    //
    // SUBJECT RULES
    //
    // The subject contains a succinct description of the change.
    'subject-case': [
      2, // Level: Error
      'always',
      [
        'lower-case', // Enforce that the subject starts with a lowercase letter.
      ],
    ],
    // The subject must not be empty.
    'subject-empty': [2, 'never'],
    // The subject must NOT end with a period.
    'subject-full-stop': [2, 'never', '.'],
    // A hard limit on the subject line length.
    // Git's recommendation is a soft limit of 50, but 72 is a common hard limit.
    'subject-max-length': [2, 'always', 72],

    //
    // BODY & FOOTER RULES
    //
    // The body should include the motivation for the change and contrast with previous behavior.
    // The footer should contain information about Breaking Changes, and is also the place
    // to reference GitHub issues, Jira tickets, etc.

    // There must be a blank line between the subject and the body.
    'body-leading-blank': [2, 'always'],
    // Enforce a maximum line length for the body for readability.
    'body-max-line-length': [2, 'always', 100],

    // There must be a blank line between the body and the footer.
    'footer-leading-blank': [2, 'always'],
    // Enforce a maximum line length for the footer.
    'footer-max-line-length': [2, 'always', 100],
  },
};