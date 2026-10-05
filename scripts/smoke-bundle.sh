#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 /path/to/golangci-lint" >&2
  exit 2
fi

binary=$1
if [[ $binary != /* ]]; then
  binary="$PWD/$binary"
fi
if [[ ! -x $binary ]]; then
  echo "golangci-lint binary is not executable: $binary" >&2
  exit 2
fi

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
workspace=$(mktemp -d)
trap 'rm -rf "$workspace"' EXIT
cd "$workspace"

cat > go.mod <<'EOF'
module smoke.local/bundle

go 1.27.0
EOF

mkdir -p anonymous inline
cat > anonymous/anonymous.go <<'EOF'
package anonymous

var bad = struct{ Value string }{Value: "bad"}
EOF

cat > inline/inline.go <<'EOF'
package inline

import "errors"

func pair() (int, error) { return 1, errors.New("failure") }
func consume(int)       {}

func broken() error {
	value, err := pair()
	if err != nil {
		return err
	}
	consume(value)
	return nil
}
EOF

set +e
"$binary" run \
  --config "$repo_root/.golangci.yml" \
  --output.json.path stdout \
  --output.text.path /dev/null \
  --show-stats=false \
  ./... > findings.json 2> findings.stderr
lint_status=$?
set -e
if [[ $lint_status -ne 1 ]]; then
  echo "expected lint findings to exit with status 1, got $lint_status" >&2
  cat findings.stderr findings.json >&2
  exit 1
fi
if ! jq -e '
  .Issues as $issues
  | ($issues | length == 2)
  and all($issues[]; .FromLinter == "justtrack")
  and (
    [$issues[] | select(
      (.Pos.Filename | endswith("anonymous/anonymous.go"))
    )] | length == 1
  )
  and (
    [$issues[] | select(
      (.Pos.Filename | endswith("inline/inline.go"))
    )] | length == 1
  )
' findings.json >/dev/null; then
  echo "expected exactly the two justtrack analyzer findings, one per fixture" >&2
  cat findings.stderr findings.json >&2
  exit 1
fi

cat > anonymous/anonymous.go <<'EOF'
package anonymous

type payload struct{ Value string }

var good = payload{Value: "good"}
EOF

cat > inline/inline.go <<'EOF'
package inline

import "errors"

func pair() (int, error) { return 1, errors.New("failure") }
func consume(int)       {}

func corrected() error {
	if value, err := pair(); err != nil {
		return err
	} else {
		consume(value)
	}
	return nil
}
EOF

set +e
"$binary" run \
  --config "$repo_root/.golangci.yml" \
  --output.json.path stdout \
  --output.text.path /dev/null \
  --show-stats=false \
  ./... > clean.json 2> clean.stderr
clean_status=$?
set -e
if [[ $clean_status -ne 0 ]]; then
  echo "expected corrected fixtures to pass, got exit status $clean_status" >&2
  cat clean.stderr clean.json >&2
  exit 1
fi
if [[ -s clean.json ]] && ! jq -e '((.Issues // []) | length) == 0' clean.json >/dev/null; then
  echo "corrected fixtures unexpectedly produced JSON findings" >&2
  cat clean.stderr clean.json >&2
  exit 1
fi

echo "custom bundle smoke test passed"
