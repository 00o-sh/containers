#!/usr/bin/env bash

export APP_DIR="/app/emby"

export AMDGPU_IDS="${APP_DIR}/extra/share/libdrm/amdgpu.ids"
export FONTCONFIG_PATH="${APP_DIR}/etc/fonts"
export OCL_ICD_VENDORS="${APP_DIR}/extra/etc/OpenCL/vendors"
export PCI_IDS_PATH="${APP_DIR}/share/hwdata/pci.ids"
export SSL_CERT_FILE="${APP_DIR}/etc/ssl/certs/ca-certificates.crt"
if [ -d "/lib/x86_64-linux-gnu" ]; then
    export LIBVA_DRIVERS_PATH="/usr/lib/x86_64-linux-gnu/dri:${APP_DIR}/extra/lib/dri"
fi
export HOME="/config"

# emby ≥ 4.9.5.0 ships its own bundled dynamic linker at
# /app/bin/lib/ld-linux-x86-64.so.2, referenced via the RELATIVE
# ELF interpreter path `lib/ld-linux-x86-64.so.2`. The kernel
# resolves a relative PT_INTERP against the current working
# directory at exec time (not the binary's location), so we have
# to chdir into /app/bin before exec'ing or the kernel returns
# ENOENT looking for the linker and bash prints
# "cannot execute: required file not found".
# Older emby (≤ 4.9.3.0) used an absolute interpreter and didn't
# need this; the cd is harmless for that case too.
cd "${APP_DIR}" || exit 1

exec \
    env --chdir="${APP_DIR}" \
        LD_LIBRARY_PATH="${APP_DIR}/lib:${APP_DIR}/extra/lib" \
        "${APP_DIR}/system/EmbyServer" \
            -programdata /config \
            -ffdetect "${APP_DIR}/bin/ffdetect" \
            -ffmpeg "${APP_DIR}/bin/ffmpeg" \
            -ffprobe "${APP_DIR}/bin/ffprobe" \
            -restartexitcode 3 \
            "$@"
