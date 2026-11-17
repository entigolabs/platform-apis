#!/bin/bash
# Mirrors the upstream Crossplane functions of helm/values.yaml (.functions) and pins them by
# sha256 digest. A pinned tag in values.yaml reads "<version>@sha256:<digest>".
#
#   pullpush.sh digests   Resolve the upstream digest of every function and write it into its tag
#                         in helm/values.yaml. Needs no credentials. Run it after changing a
#                         function's version and commit the result.
#   pullpush.sh           Copy every function to the mirrors, by the digest in its tag, and verify
#                         that each mirror serves exactly that digest. Needs a login to the mirrors.
#
# MIRRORS (space separated, default docker.io/entigolabs) lists the mirror repository prefixes.
# A function is copied byte for byte, so its digest is the same upstream and on every mirror.
set -uo pipefail

VALUES_FILE="$(dirname "$0")/helm/values.yaml"
MIRRORS="${MIRRORS:-docker.io/entigolabs}"
COMMAND="${1:-mirror}"

# Digest of an image reference, empty when it does not exist.
digest_of() {
    docker buildx imagetools inspect "$1" --format '{{.Manifest.Digest}}' 2>/dev/null | grep -E '^sha256:[0-9a-f]{64}$'
}

# "<image> <tag>" of every function, the tag as written in values.yaml.
functions() {
    yq eval '.functions[] | .image + " " + .tag' "$VALUES_FILE"
}

write_digests() {
    local image tag version digest
    while read -r image tag; do
        version=${tag%%@*}
        digest=$(digest_of "$image:$version")
        if [ -z "$digest" ]; then
            echo "No digest found for $image:$version" >&2
            exit 2
        fi
        # Only the tag line that follows this function's image line, so the file keeps its layout.
        sed -i "\|^    image: $image\$|{n;s|^    tag: .*\$|    tag: $version@$digest|}" "$VALUES_FILE"
        echo "$image:$version@$digest"
    done < <(functions)
}

mirror() {
    local image tag version digest mirror dest found
    while read -r image tag; do
        version=${tag%%@*}
        digest=${tag#"$version"}
        digest=${digest#@}
        if [ -z "$digest" ]; then
            echo "$image:$version has no digest in $VALUES_FILE, run '$0 digests' and commit the result" >&2
            exit 2
        fi
        for mirror in $MIRRORS; do
            dest="$mirror/${image##*/}:$version"
            found=$(digest_of "$dest")
            if [ -z "$found" ]; then
                echo "Copying $image@$digest to $dest"
                if ! docker buildx imagetools create --tag "$dest" "$image@$digest"; then
                    echo "Failed to copy $image@$digest to $dest" >&2
                    exit 2
                fi
                found=$(digest_of "$dest")
            else
                echo "Skipping - already exists at $dest"
            fi
            if [ "$found" != "$digest" ]; then
                echo "$dest is $found, values.yaml records $digest" >&2
                exit 2
            fi
        done
    done < <(functions)
    echo "All functions copied successfully"
}

case "$COMMAND" in
    digests) write_digests ;;
    mirror) mirror ;;
    *) echo "Usage: $0 [mirror|digests]" >&2; exit 1 ;;
esac
