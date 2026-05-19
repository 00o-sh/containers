# syntax=docker/dockerfile:1

FROM docker.io/library/ubuntu:24.04
ARG TARGETARCH
ARG VERSION

ENV DEBIAN_FRONTEND="noninteractive" \
    NVIDIA_DRIVER_CAPABILITIES="compute,video,utility" \
    ATTACHED_DEVICES_PERMS="/dev/dri /dev/dvb /dev/vchiq /dev/video1? -type c"

USER root

RUN \
    apt-get update \
    && \
    apt-get install -y --no-install-recommends --no-install-suggests \
        bash \
        ca-certificates \
        catatonit \
        curl \
        jq \
        nano \
        tzdata \
    && \
    curl -fsSL -o /tmp/emby.deb \
        "https://github.com/MediaBrowser/Emby.Releases/releases/download/${VERSION}/emby-server-deb_${VERSION}_${TARGETARCH}.deb" \
    && \
    # Install whatever emby's .deb declares as Depends. We use `dpkg-deb -xv`
    # below to extract (skipping postinst), so apt isn't resolving these for
    # us automatically. Doing it here auto-tracks new deps across emby
    # releases — the static list approach in #27 caught 4.9.3.0 but not
    # 4.9.5.0 because upstream added more runtime libs. Strip version
    # constraints in parens and pick the first alternative for `pkg1 | pkg2`.
    apt-get install -y --no-install-recommends --no-install-suggests \
        $(dpkg-deb -f /tmp/emby.deb Depends \
            | tr ',' '\n' \
            | sed -e 's/([^)]*)//g' -e 's/|.*$//' \
            | xargs) \
    && \
    mkdir -p /app/bin /tmp/emby \
    && dpkg-deb -xv /tmp/emby.deb /tmp/emby/ \
    && mv -t /app/bin/ /tmp/emby/opt/emby-server/* \
    && chown -R root:root /app && chmod -R 755 /app \
    && apt-get purge -y --auto-remove -o APT::AutoRemove::RecommendsImportant=false \
    && apt-get autoremove -y \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

COPY . /

USER nobody:nogroup
WORKDIR /config
VOLUME ["/config"]

ENTRYPOINT ["/usr/bin/catatonit", "--", "/entrypoint.sh"]
