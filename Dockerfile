FROM golang:1.19-alpine as builder

WORKDIR /go/src/github.com/matutter/upsmon/
COPY cmd cmd
COPY pkg pkg
ADD go.mod go.mod
ADD go.sum go.sum
ADD go.work go.work

RUN \
  apk add libusb-dev libc-dev gcc && \
  cd cmd/server && \
  CGO_ENABLED=1 go build -a -installsuffix cgo -o ../../upsmon-server .

FROM alpine:3.15

ENV \
  UPS_CONFIG="/etc/upsmon/upsmon.yml"

EXPOSE 8080

WORKDIR /root/
COPY --chown=0:0 config/dist.yml /etc/upsmon/upsmon.yml
COPY --from=builder --chown=0:0 /go/src/github.com/matutter/upsmon/upsmon-server ./
CMD [ "./upsmon-server" ]

RUN apk --no-cache add libusb
