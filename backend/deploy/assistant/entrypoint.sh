#!/bin/sh
# Start the assistant: put the repo's config and persona in place, then run the gateway in the foreground.
set -eu
cp /assistant/config.yaml /opt/data/config.yaml
cp /assistant/SOUL.md /opt/data/SOUL.md
exec /opt/hermes/bin/hermes gateway run
