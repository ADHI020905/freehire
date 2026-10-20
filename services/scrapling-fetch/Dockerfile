# The fetch service on Scrapling's own image, pinned by digest: the service reaches into
# Scrapling's session API, and a floating tag would change it without a diff here. That image
# already carries Chromium, curl_cffi, starlette and uvicorn, so nothing is installed on top.
FROM ghcr.io/d4vinci/scrapling@sha256:1d876879813759959bc4ba6fbb1ce8247ebbc2b67eee77b140baf7c80af6ae6b

COPY app.py engines.py pool.py main.py /service/
WORKDIR /service

# The base image installs its browsers under /root/.cache/ms-playwright, so the browser has
# to run as root; the container, not the user inside it, is the boundary here.
# hadolint ignore=DL3002
USER root
ENTRYPOINT ["/app/.venv/bin/python", "/service/main.py"]
