# Branch Ownership

Per [TEAM-POLICIES.md §6.1](TEAM-POLICIES.md#6-version-control), every branch is owned by at least two team members. `main` is owned by all four.

| Branch                     | Owners                        | Scope                     |
|----------------------------|-------------------------------|---------------------------|
| `main`                     | Heffelmire, Jones, LaGoy, Mao | Entire repository         |
| `server`                   | Heffelmire, Jones, LaGoy, Mao | `server/`, `proto/`       |
| `chess-client-compiled`    | Heffelmire, Mao               | `games/chess/client-1/`   |
| `chess-client-interpreted` | Jones, LaGoy                  | `games/chess/client-2/`   |
| `go-fish-client-1`         | Heffelmire, LaGoy             | `games/gofish/client-1/`  |
| `go-fish-client-2`         | Jones, Mao                    | `games/gofish/client-2/`  |
| `tic-tac-toe`              | Heffelmire, Jones, LaGoy, Mao | `games/tictactoe/client/` |

Update this table whenever a branch's owners change, and open a pull request against it per §6.1 so the change is reviewed like any other.
