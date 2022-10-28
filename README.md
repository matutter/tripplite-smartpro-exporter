# UPS Monitor

A UPS monitor that supports watching metrics over HTTP.

Currently only support TrippLite SMARTPRO UPS using the USB 3003 protocol.

## Environment Variables

- `UPS_LISTEN` default: `0.0.0.0:8080`
- `UPS_DEBUG` default: `false`
- `UPS_VENDOR_ID` default: `""`
- `UPS_PRODUCT_ID` default: `""`
- `UPS_DELAY` default: `5s`
- `UPS_HISTORY_SIZE` default: `1000`
- `UPS_HMAC_SECRET` default: `""`
- `UPS_CONFIG` default: `"/etc/upsmon/upsmon.yml"`

## Dev Notes

Build and run docker:

```bash
make docker &&  docker run -it --rm \
  -v `pwd`/config/debug.yml:/etc/upsmon/upsmon.yml:ro \
  --device /dev/bus/usb/002/019 \
  --read-only \
  upsmon:1.0.0
```

Run from source:

```bash
sudo go run cmd/server/main.go
(cd cmd/server; UPS_CONFIG=../../config/debug.yml sudo go run .)
```

```bash
(cd pkg/tripplite/; go test -v)
(cd cmd/server/; go test -v)
```

Add the following lines to `/etc/sudoers` to pass `UPS_*` environment variables:

```bash
Defaults        env_reset
Defaults        env_keep += "GO*"
Defaults        env_keep += "UPS_*"
```

## Network UPS Tools

This project was made to make for fun and takes a lot of the Tripplite USB code
from [NUT](https://networkupstools.org/).
