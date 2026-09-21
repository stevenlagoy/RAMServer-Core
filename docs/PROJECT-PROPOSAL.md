# Project Proposal

**Project:** Reusable Authoritative Multiplayer Server Core (RAMServer)

**Course:** CS 56000 Software Engineering

**Term:** Fall 2026

#### Team:
- Heffelmire, Jacob : [heffjl03@pfw.edu](mailto:heffjl03@pfw.edu) / [jlheffelmire@gmail.com](mailto:jlheffelmire@gmail.com)
- Jones, Hayden : [jonehm05@pfw.edu](mailto:jonehm05@pfw.edu) / [jonehayhayd@gmail.com](mailto:jonehayhayd@gmail.com)
- LaGoy, Steven : [lagosm01@pfw.edu](mailto:lagosm01@pfw.edu) / [stevenlagoy@gmail.com](mailto:stevenlagoy@gmail.com)
- Mao, Aidan : [maoal01@pfw.edu](mailto:maoal01@pfw.edu) / [aidanmao2005@gmail.com](mailto:aidanmao2005@gmail.com)

## Problem Statement

Student and hobbyist multiplayer game projects are frequently built with client-authoritative logic: each client tracks its own copy of the game state and simply reports its actions to peers or a thin relay server. This approach is fast to prototype but is trivially exploitable (a client can claim any state it wants) and is rebuilt from scratch for every new game, since the networking, validation, and synchronization logic is tightly coupled to that one game's rules. Teams that want to build a second game, or a game with real anti-cheat guarantees, end up re-solving the same authoritative-server problem with no reusable foundation to build on.

## Proposed Solution and Goal

We propose a standalone, game-agnostic, authoritative multiplayer server core. Client actions are submitted as requests, validated against the current game state on the server, applied only if legal, and then broadcast to all connected clients as the single source of truth. The core exposes a game-agnostic interface (including state representation, legal-move checking, and win conditions) so that a specific turn-based game can be implemented as a plugin without modifying the networking, validation, or persistence layers. Success looks like: two independently written clients, in two different languages, connect to the same running server and complete a full match of a simple turn-based game, with the server correctly rejecting at least one deliberately illegal or spoofed client action during testing.

## Stakeholders and Users

Primary users: Players connecting via a client application to play a turn-based game against another player, with confidence that neither side can cheat by falsifying game state.

Secondary stakeholders: Developers who want to build a new turn-based game on top of an existing, tested authoritative server core rather than rebuilding networking and anti-cheat logic from scratch.

## Initial Scope

### Core features (required for the minimum viable product):

- Authoritative server core with a game-agnostic state/validation interface

- Client-to-server action submission and server-to-client state broadcast over a documented wire protocol

- One reference game implemented against the interface (such as tic-tac-toe, a card game, checkers or chess, or grid-based strategy game)

- Two independently written clients in different languages (one compiled and one interpreted) connecting to the same server

- Match result and action-log persistence for completed matches

### Optional features (only after core requirements are implemented and tested):

- A second pluggable game demonstrating true game-agnosticism of the core

- Reconnection/session-resume support after a dropped client connection

- Deployment to a cloud-hosted instance for a real-network (non-LAN) demonstration

### Out-of-scope:

- Matchmaking/lobbies beyond a simple direct-connect flow

- Graphical polish or art assets beyond what is needed to demonstrate gameplay

- Support for intensive, real-time games, like first-person shooters or racing games.

## Initial Feature List

- The system shall validate every player action against the current authoritative game state before applying it.

- The system shall reject and log any client-submitted action that is illegal given the current game state.

- The system shall broadcast the updated game state to all connected clients immediately after a validated action is applied.

- The system shall expose a documented wire protocol such that a client implemented in a different programming language than the reference client can connect and play a full match.

- The system shall define a game-agnostic interface such that a new game can be added without modifying the core networking, validation, or persistence layers.

- The system shall persist the result and full action log of each completed match.

- The system shall authenticate a connecting client before allowing it to submit actions for a match.

## Constraints and Feasibility

- **Time:** 13 weeks remain in the semester as of this proposal; the team has scoped the core and one reference game as the minimum viable product, with a second game and cloud deployment held as optional stretch goals.

- **Skills:** the team has prior experience with Java and LibGDX from other projects, which de-risks client development and lets more time be spent on the server core, which is the project's primary learning objective.

- **Technology:** server core in Go; wire protocol serialized with Protocol Buffers over TCP; clients in Java (LibGDX) and Python (Pygame); match persistence via embedded SQLite; no third-party multiplayer game API or engine networking layer will be used, since implementing the authoritative-server logic is the point of the project.

- **Infrastructure:** development and LAN testing require no paid resources; a cloud-hosted demonstration, if pursued, will use the team's existing Google Cloud student credits for a single small virtual machine.

- **Data/privacy:** the system stores only gameplay data (match actions and results); no personal or sensitive data is collected.

## Software Process

The team will use an Agile hybrid process: a shared kanban board (GitHub Projects) tracking tasks through backlog, in-progress, review, and done, with informal standups and progress checks rather than fixed-length sprints. This fits a 4-person team working across three languages, where task dependencies (for example, a client change waiting on a protocol update) are better tracked continuously than batched into sprint boundaries. The team will hold brief standups at each regular meeting to surface blockers, and will treat the two scheduled course stand-ups as checkpoints for reassessing scope.

## Team Plan

Roles: Each team member will have one of the following primary roles:

- **Project Manager**
    - Scopes work items and makes initial task assignments
    - Schedules and conducts planning meetings; publishes agendas
    - Maintains project vision & requirements model
    - Reviews documentation for completeness and accuracy
- **Protocol Designer**
    - Specifies the wire protocol for client-server communication
    - Produces protocol documentation sufficient for conforming clients
- **Network Engineer**
    - Designs and implements the transport and session layers
    - Manages connection lifecycle, framing, timeouts, etc
    - Defines reconnection and session-resume behavior (stretch goal)
- **Quality and Release Engineer**
    - Owns CI/CD pipeline and the Docker environment definition
    - Defines test strategy, required test tiers, and merge criteria

Along with these primary roles, all team members will be server developers, and each will be in one of two client developer pairs; per course requirements, all members will maintain working knowledge of the full system, and roles may be rebalanced as the project progresses.

**Communication:** Discord for day-to-day coordination; weekly virtual stand-up meetings on Fridays / according to team availability; pair or group programming ad hoc. Branches will have assigned owners and all owners must approve pull requests into their branches, all four members will be owners of the main branch (PRs with main require approval of all group members).

**Documentation:** Documentation will be done in the source code through comments, as readme markdown files in the project repository, and in Google Docs accessible through links in the repository.

**Project Management:** A hybrid-agile approach will be used, with Kanban used for tracking tasks in the absence of an official sprint schedule. GitHub Projects and Issues will be used to specify and update tasks.

**Conflict procedure:** Disagreements on technical decisions will be raised and discussed at the next team meeting; unresolved disagreements affecting scope or schedule will be brought to the instructor.

## Preliminary schedule (liable to change):
- **Weeks 4–6:** Protocol design and core state machine
- **Weeks 7–9:** Rule enforcement engine and first client in Java & LibGDX or C++ & Raylib using the protocol
- **Weeks 10–12:** Second client in Python & Pygame or Lua & LÖVE using the same protocol; persistence layer with an embedded SQLite database
- **Weeks 13–15:** Testing including deliberate illegal-action and dropped-connection cases, optional stretch goals
- **Week 16:** Final report, presentation, and demonstration

## Tools and References

- **VCS:** GitHub
- **Project Management:** GitHub Projects
- **CI/CD:** GitHub Actions
- **Server language/runtime:** Go
- **Wire protocol:** Protocol Buffers over TCP
- **Client 1:** Compiled language (ex: Java with LibGDX, C++ with Raylib)
- **Client 2:** Interpreted language (ex: Python with Pygame, Lua with LÖVE)
- **Persistence:** embedded SQLite
- **Containerization:** Docker
- **Cloud resources (optional deployment):** Google Cloud Platform
