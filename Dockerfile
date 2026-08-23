# Copyright (c) 2026 ChatNet (sh). MIT licensed, see LICENSE.
#
# Herald -- ChatNet's GitHub announcement bot (see main.go's own header
# comment, and github.com/Libera-Chat/rutile for the project that
# inspired it). Hand-rolled IRC client over stdlib only, same as every
# other bot in this project -- no IRC library, no third-party webhook
# framework either (net/http is enough for one endpoint).

FROM golang:1.22-alpine AS build

WORKDIR /app
COPY go.mod ./
COPY main.go irc.go webhook.go ./
RUN go build -o /opt/herald/herald .

FROM alpine:latest

RUN adduser -D -h /home/herald herald

COPY --from=build /opt/herald/herald /opt/herald/herald

USER herald
WORKDIR /home/herald
ENTRYPOINT ["/opt/herald/herald"]
