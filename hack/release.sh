#!/usr/bin/env bash
set -euo pipefail

usage() {
    echo "usage: $0 <vX.Y.Z | major | minor | patch>" >&2
    exit 1
}

[ $# -eq 1 ] || usage

cd "$(git rev-parse --show-toplevel)"

if [ "$(git rev-parse --abbrev-ref HEAD)" != master ]; then
    echo "not on master" >&2
    exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
    echo "working tree is not clean" >&2
    exit 1
fi

git fetch --quiet origin master --tags
if [ "$(git rev-parse HEAD)" != "$(git rev-parse origin/master)" ]; then
    echo "master and origin/master differ, push or pull first" >&2
    exit 1
fi

last=$(git tag --list 'v*' --sort=-v:refname | head -n1)
IFS=. read -r major minor patch <<<"${last#v}"
major=${major:-0} minor=${minor:-0} patch=${patch:-0}

case $1 in
    major) version="v$((major + 1)).0.0" ;;
    minor) version="v${major}.$((minor + 1)).0" ;;
    patch) version="v${major}.${minor}.$((patch + 1))" ;;
    v[0-9]*.[0-9]*.[0-9]*) version=$1 ;;
    *) usage ;;
esac

if git rev-parse -q --verify "refs/tags/$version" >/dev/null; then
    echo "tag $version already exists" >&2
    exit 1
fi

go test ./...

read -r -p "Tag $(git rev-parse --short HEAD) as $version (last: ${last:-none}) and push? [y/N] " answer
[ "$answer" = y ] || exit 1

git tag -a "$version" -m "$version"
git push origin "$version"
