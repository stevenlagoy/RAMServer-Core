# Team Policies

**Project:** Reusable Authoritative Multiplayer Server Core (RAMServer)

**Course:** CS 56000 Software Engineering

**Term:** Fall 2026

#### Team:
- Heffelmire, Jacob : [heffjl03@pfw.edu](mailto:heffjl03@pfw.edu) / [jlheffelmire@gmail.com](mailto:jlheffelmire@gmail.com)
- Jones, Hayden : [jonehm05@pfw.edu](mailto:jonehm05@pfw.edu) / [jonehayhayd@gmail.com](mailto:jonehayhayd@gmail.com)
- LaGoy, Steven : [lagosm01@pfw.edu](mailto:lagosm01@pfw.edu) / [stevenlagoy@gmail.com](mailto:stevenlagoy@gmail.com)
- Mao, Aidan : [maoal01@pfw.edu](mailto:maoal01@pfw.edu) / [aidanmao2005@gmail.com](mailto:aidanmao2005@gmail.com)

## 1. Purpose and Scope

This document defines the working agreements for the team for the duration of the project. It covers roles, communication, task tracking, version control, code review, coding and testing standards, environment management, documentation, and conflict resolution.

These policies apply to all team members and to all work products in the project repository. Where a policy conflicts with a course requirement or an instructor directive, the course requirement takes precedence and this document is amended to match.

## 2. Roles and Responsibilities

Every member holds exactly one primary role and two shared roles. Primary roles denote ownership of a decision area, not exclusive access to the work. Per course requirements, all members maintain working knowledge of the full system.

### 2.1 Primary roles (one member each)

#### Project Manager
- Scopes work items and makes initial task assignments.
- Schedules and conducts planning meetings; sets and publishes agendas.
- Maintains the project vision, the requirements model, and the traceability between requirements and open issues.
- Reviews all documentation for completeness and accuracy before a deliverable is submitted.
- Owns this policy document and the course deliverable calendar.

#### Protocol Designer
- Specifies the wire protocol used for client-server communication, including message schemas, versioning rules, and error semantics.
- Owns the .proto definitions and is a required reviewer on any pull request that modifies them.
- Produces and maintains protocol documentation sufficient for a developer to write a conforming client without reading the server source.
- Announces breaking protocol changes in the team Discord before the corresponding pull request is opened.

#### Network Engineer
- Designs and implements the transport and session layer between server and clients, including connection lifecycle, framing, timeouts, and disconnect handling.
- Owns the server-side networking module and the shared client-side transport concerns.
- Defines the reconnection and session-resume behavior if that stretch goal is pursued.

#### Quality and Release Engineer
- Owns the CI/CD pipeline configuration in .github/workflows/ and the Docker environment definitions (Dockerfile, docker-compose.yml).
- Defines the test strategy, the required test tiers, and the merge gate criteria in §9.
- Maintains test fixtures and the adversarial test suite covering illegal, spoofed, and out-of-order client actions.
- Owns the persistence schema and its migrations, since match and action-log integrity is verified through the same test harness.

### 2.2 Shared roles (all members)

#### Server Developer (all four members)
- Collectively own and develop the server core: state representation, validation interface, rule enforcement, and persistence.
- No single member is the sole author of any server subsystem. Each subsystem has at least two members familiar with it.

#### Client System Developer (all four members, in two pair-programming groups)
- Each pair owns one client implementation: one compiled-language client and one interpreted-language client.
- Each pair interprets the published protocol specification into a language-specific implementation independently, without reusing the other pair's code.
- Independent implementation is a project goal: it is the evidence that the protocol is genuinely language-agnostic. Pairs should resolve ambiguity by consulting the protocol specification and the Protocol Designer rather than the other pair's source.

## 3. Communication

1) Day-to-day coordination takes place in the team Discord group.

2) Course deliverables are circulated by email to all team members before submission. The email includes the deliverable, the submission deadline, and a request for objections. Submission proceeds if no objection is raised before the deadline.

3) Decisions of record (protocol changes, scope changes, technology changes) are posted in Discord and recorded in the repository, either in a meeting-notes document or in the relevant design document. A decision that exists only in a direct message has not been made.

4) Response expectation: members acknowledge direct mentions within 48 hours during the working week.

5) Planned absence: a member who will be unreachable for more than 48 hours posts notice in advance on Discord, including the state of their in-progress work.

6) Unplanned absence: a member unreachable for more than 3 consecutive days may have their in-progress tasks reassigned by the Project Manager. Reassignment is announced on Discord and reflected on the kanban board. Reassignment is a schedule-protection measure and carries no penalty to the absent member.

## 4. Meetings

| Meeting                    | Cadence                                          | Duration   | Led by          |
|----------------------------|--------------------------------------------------|------------|-----------------|
| Virtual standup            | Weekly, Friday 6:00PM                            | 30 min     | Project Manager |
| Planning / scope review    | As scheduled, at minimum once per phase boundary | 60 min     | Project Manager |
| Course standup checkpoints | Per course schedule (2 total)                    | Per course | Project Manager |

Standup format. Each member reports: work completed since the last standup, work planned before the next, and any blocker. Blockers are converted into kanban items or assigned an owner before the meeting ends. Discussion that involves fewer than all four members is deferred to a follow-up conversation.

Attendance. A member who cannot attend posts a written standup on Discord before the meeting begins.

Notes. The Project Manager records decisions, action items, and owners in the shared meeting-notes document, which is linked from the repository README.

## 5. Task Tracking

1) All work is tracked as a GitHub Issue in the project repository. Work that is not on the board is not tracked and cannot be counted as a contribution.

2) Issues are organized on the project's GitHub Projects kanban board with the columns: Backlog, Ready, In Progress, In Review, and Done.

3) Every issue has an assigned owner. Issues sit in Backlog only while unowned; an issue moved to Ready has an owner.

4) Only the issue owner may change an issue's status. Any member may comment on, add detail to, or link work to any issue.

5) The board is kept current at all times. Members update issue status when the status changes rather than in a batch before a meeting.

6) Issues are labeled by subsystem (server, protocol, client-1, client-2, persistence, infra, docs) and by type (feature, bug, test, chore).

7) Issues are scoped so that a single issue is completable by one member within roughly one week. Larger work is decomposed by the Project Manager in consultation with the owner.

8) Each issue states its acceptance criteria before it moves to Ready.

## 6. Version Control

### 6.1 Branch ownership

1) Every branch is owned by at least two team members. Ownership is recorded in the issue that the branch serves and in docs/branch-ownership.md.

2) The main branch is owned by all four members.

3) Branch owners are responsible for the state of their branch, including resolving merge conflicts on it. Merge conflicts are resolved by the owners of the target branch, in consultation with the author of the incoming change when the conflict involves logic rather than formatting.

### 6.2 Branch organization

Feature-branch organization is used. Each feature or issue receives a dedicated branch.

#### Naming convention:

`<type>/<issue-number>-<short-kebab-description>`

Examples:
- `feature/42-illegal-action-rejection`
- `fix/57-tcp-frame-boundary`
- `test/61-spoofed-action-suite`
- `docs/63-protocol-spec-v2`
- `chore/70-dockerfile-multistage`

Branches are deleted after they merge.

### 6.3 Pull requests

1) All changes reach a protected branch through a pull request. Direct pushes to main are disabled in repository settings.

2) A pull request into any branch requires approval from all owners of the target branch. A pull request into main therefore requires approval from all four members.

3) Every approval includes a written comment stating the reviewer's reason for approving, at minimum one sentence. Approving with an empty comment is not a valid approval under this policy. A reason identifies what the reviewer checked, for example: "Verified that the rejection path logs the offending action and that the new table-driven test covers the out-of-turn case."

4) A pull request that changes the wire protocol requires the Protocol Designer among its reviewers, regardless of target branch.

5) A pull request title is descriptive and references its issue, for example: Reject out-of-turn actions and log rejection reason (#42).

6) A pull request description states what changed, why, how it was tested, and any follow-up work left open.

7) Review turnaround target: 48 hours during the working week. A reviewer who cannot meet that window says so in the pull request thread.

### 6.4 Merging into `main`

A merge into `main` occurs when a sufficient, stable product increment is complete. The increment must:
- satisfy the acceptance criteria of its issue;
- pass all existing automated tests in CI;
- carry approval, with written reasons, from all four owners of `main`;
- leave `main` in a runnable state, meaning the server builds and starts and the existing clients connect.

Squash merges are used for feature branches so that `main` history reads as one commit per completed increment.

### 6.5 Commit standards

1) A commit pertains to a single system or feature. Changes to unrelated subsystems belong in separate commits. This keeps changes traceable and keeps commit size consistent enough to compare contributions across members meaningfully.

2) Commits are regular. Members commit at natural stopping points rather than accumulating a day of work into one change.

3) Commit messages are descriptive and written in the imperative mood, with a subject line under 72 characters and an optional body explaining the reasoning.

#### Commit Message Example:
```
Add turn-order validation to the move validator \
Rejects actions submitted by a player whose turn is not current. \
Rejection reason is written to the action log so that the client receives a specific error rather than a generic refusal. \
Refs #42
```

Commit messages such as 'fix', 'wip', 'update', or 'asdf' are not acceptable on any branch that will be reviewed.

## 7. Coding Standards

1) Each language follows its own community standard, applied through automated tooling in CI:

| Language         | Style standard                                          | Tooling                      |
|------------------|---------------------------------------------------------|------------------------------|
| Go               | Effective Go; standard library idioms                   | gofmt, go vet, golangci-lint |
| Java             | Google Java Style or equivalent, agreed at design phase | Checkstyle or Spotless       |
| Python           | PEP 8, PEP 257                                          | ruff or flake8 + black       |
| Protocol Buffers | Buf style guide                                         | buf lint                     |

2) Naming of modules, files, types, and variables follows the conventions of the language in use. Names carry meaning; abbreviations are avoided except for widely understood ones.

3) Design follows the SOLID principles. Of these, the Dependency Inversion and Interface Segregation principles carry particular weight here, since the game-agnostic core depends on a game interface rather than on any concrete game.

4) The DRY principle applies within each codebase. It does not apply across the two client implementations, where independent implementation is a deliberate design goal [§2.2](#22-shared-roles-all-members).

5) Well-known design patterns are used where they fit the problem, and are named in code comments or design documentation when used. Patterns are not applied for their own sake; a pattern that adds indirection without solving a present problem is rejected in review.

6) Public interfaces carry documentation comments in the language's standard form (godoc, Javadoc, docstrings).

7) Formatting and linting run in CI and are a merge gate. Style disagreements are settled by the configured tool rather than in review comments.

## 8. Environment and Build

1) Docker provides a consistent development, test, and demonstration environment. A developer should be able to clone the repository and bring up the server and both clients with a single documented command.

2) Docker practice:
    - Pin base image versions; avoid latest.
    - Use multi-stage builds to keep runtime images small.
    - Run containers as a non-root user.
    - Maintain a .dockerignore file.
    - Express service topology for local development in docker-compose.yml.
    - Keep build context minimal and layer ordering cache-friendly.

3) Configuration is supplied by environment variable, not hard-coded. No credentials, keys, or tokens are committed to the repository.

4) CI runs on GitHub Actions for every pull request and every push to main. The pipeline builds all three components, runs linters, and runs the full test suite. A failing pipeline blocks merge.

## 9. Testing and Verification

1) Testing is a first-order obligation of this project, not a closing activity.

2) Every member writes tests for the systems they implement. A pull request that adds behavior without adding or updating tests is returned in review.

3) A merge into main must pass all existing tests. No exceptions, including near deadlines. A test that is failing for a known, accepted reason is fixed or explicitly removed with team agreement; it is not skipped silently.

4) Tests encode the system's constraints and assumptions as directly as the language allows. Where a design assumption cannot be expressed as a test, it is documented in the relevant design document.

5) Required test tiers:
    - Unit tests for validation logic, state transitions, and protocol encoding and decoding.
    - Integration tests for the full request-validate-apply-broadcast cycle against a running server instance.
    - Adversarial tests covering illegal actions, out-of-turn actions, spoofed identity, malformed frames, and replayed messages. The server is expected to reject and log each of these.
    - Cross-language conformance tests verifying that both clients complete a full match against the same server build.
6) Bug fixes are accompanied by a regression test that fails before the fix and passes after it.
7) The Quality and Release Engineer maintains the test strategy document and reports suite health at standup.

## 10. Documentation

1) Code comments explain intent and non-obvious reasoning. Comments that restate the code are removed in review.

2) README files live at the repository root and in each major subdirectory (server/, clients/client-1/, clients/client-2/, proto/). The root README covers project purpose, prerequisites, build and run instructions, and links to all other documentation.

3) Google Docs are used where collaborative drafting or course formatting requires them. Every such document is linked from the repository, and the repository is the index of record. A document that is not linked from the repository does not exist for the team's purposes.

4) The protocol specification is versioned in the repository alongside the .proto definitions and is updated in the same pull request as any protocol change.

5) The Project Manager reviews documentation before each course deliverable.

## 11. Conflict Resolution

Conflicts are escalated in three stages:

1) Direct conversation. The two members involved discuss the matter directly and attempt to resolve it between themselves.

2) Team involvement. If no resolution is reached, the matter is brought to the full team. Technical disagreements are decided at the next team meeting, by consensus where possible and by majority vote where not. The Project Manager breaks a tie. The decision and its reasoning are recorded.

3) Instructor involvement. If the team cannot reach a resolution, or if the disagreement affects scope, schedule, or the ability of the team to deliver, the course instructor is contacted.

Disagreements are recorded without attribution of fault. The record exists so the team can revisit the reasoning behind a decision later, not to assign blame.

## 12. Contribution Visibility

Contribution is evidenced by issues owned and closed, commits authored, pull requests opened, and reviews given with written reasons. The commit and issue policies in §5 and §6.5 exist in part so that this evidence is consistent and comparable across members. Members whose contribution is largely design, protocol specification, or review record that work as issues and documents so that it is visible in the same record as code.

## 13. Amending This Document

Any member may propose an amendment by opening a pull request against docs/TEAM-POLICIES.md. Amendments follow the standard main review rules, which means all four members approve with written reasons. The version number and ratification date at the head of this document are updated with each amendment.

## 14. Open Items

The following require a team decision before this document is ratified.

1) Fourth primary role (§2.1). Quality and Release Engineer is recommended, since CI, Docker, the test strategy, and the persistence schema all lack a named owner and the project's verification goals depend on them. Alternatives worth considering:
    - Game Rules Engineer, owning the reference game plugin and serving as the first consumer of the game-agnostic interface, which tests that interface for genuine reusability.
    - Documentation and Requirements Lead, owning the requirements model and all written deliverables, which reduces the Project Manager's load.

2) Reference game selection, to be finalized during the design phase (per the proposal).

3) Java style standard, Google Java Style or an alternative (§7).

4) Ratification date and signatures.
