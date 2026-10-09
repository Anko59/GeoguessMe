#!/usr/bin/env bash
# Internal raw-snapshot runner, shared by recovery and the real permission probe.
# DAC_OVERRIDE permits root to write the operator-owned 0700 output directory;
# filesystem read-only mounts still prevent source and root-filesystem writes.
snapshot_container() {
    local tool_image=$1 source_mount=$2 destination=$3
    shift 3
    docker run --rm --network none --read-only --user 0:0 --cap-drop ALL \
        --cap-add DAC_OVERRIDE --cap-add CHOWN --security-opt no-new-privileges:true \
        --mount "$source_mount" --mount "type=bind,src=$destination,dst=/snapshot" \
        "$tool_image" "$@"
}
snapshot_legacy_store() {
    local tool_image=$1 source_mount=$2 destination=$3 uid=$4 gid=$5
    # Variables below intentionally expand in the container shell, not the host.
    # shellcheck disable=SC2016
    snapshot_container "$tool_image" "$source_mount" "$destination" \
        sh -ec 'umask 077; tar -czpf /snapshot/legacy.tar.gz -C /source .; cd /snapshot; sha256sum legacy.tar.gz > legacy.tar.gz.sha256; sha256sum -c legacy.tar.gz.sha256; chmod 0600 legacy.tar.gz legacy.tar.gz.sha256; chown "$1:$2" legacy.tar.gz legacy.tar.gz.sha256' \
        _ "$uid" "$gid"
}
