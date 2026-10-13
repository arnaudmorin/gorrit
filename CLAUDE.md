# gorrit

Terminal UI for Gerrit, in Go with Bubble Tea. Talks to the Gerrit REST API.

## Layout

- `cmd/gorrit`: entry point and first-run setup prompt.
- `internal/config`: TOML config at `~/.config/gorrit/config.toml`, see `config.example.toml`.
- `internal/gerrit`: HTTP client and JSON types. Authenticated calls use the `/a/` prefix and basic auth; every JSON body starts with `)]}'`. Change ids are `<project>~<number>` with the project URL-encoded (`gerrit.ChangeID`).
- `internal/ui`: Bubble Tea models. `list.go` is the change list with filter tabs, `detail.go` the change view (summary and diff pages, keys), `summary.go` and `diffpage.go` render those pages, `compose.go` posts replies, comments and messages, `vote.go`, `download.go`, `diff.go` (external viewer), `complete.go` (search completion).

## Commands

```sh
make build        # binary at ./gorrit
make test         # unit tests
GORRIT_TEST_URL=https://review.opendev.org go test ./internal/ui/   # also run live tests, anonymous
```

## Conventions

- Keep the user's global rules: no narrating comments, short commit messages, trailer `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.
- Do not push. Commit only when asked.
- Writes to Gerrit (replies, votes, stars) are tested against a `httptest` fake server, never against a real change.
- To look at the UI, run the snapshot tests with `GORRIT_SNAPSHOT_DIR=<dir>` and convert the `.ansi` files with `python3 -I hack/ansi2png.py in.ansi out.png`, then view the PNG. Styling changes should be checked this way. The README screenshots in `docs/` are made the same way.
- Keys are listed in README.md; update it when they change.
- Columns in the change list are fixed to Code-Review, Verified and Workflow.
