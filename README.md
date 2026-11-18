# herald-bot

GitHub announcement IRC bot, inspired by Libera Chat's
[Rutile](https://github.com/Libera-Chat/rutile). Runs a small HTTP
server that receives GitHub webhook deliveries (push, pull request,
issue, and release events), verifies each one's signature, and
announces it on IRC. Which repositories get announced is decided
entirely by which ones have a webhook pointed at this bot -- there's no
repo list to maintain here.

## Requirements

- Go 1.22+ (or just Docker)
- A registered NickServ account for the bot to identify with
- An HTTPS-reachable path to this bot's `/webhook` endpoint (e.g. via a
  reverse proxy) to actually receive GitHub's deliveries -- this bot
  itself only listens on a plain internal HTTP address

## Run

```sh
docker build -t herald-bot .
docker run -d --name herald-bot \
  -e HERALD_NICKSERV_PASSWORD=... \
  -e HERALD_WEBHOOK_SECRET=... \
  herald-bot
```

Then, per repo you want announced, add a webhook under that repo's
Settings -> Webhooks: payload URL pointing at your reverse-proxied
`/webhook` path, content type `application/json`, secret matching
`HERALD_WEBHOOK_SECRET`, and select the push/pull request/issues/release
events (or "send me everything", the bot ignores event types it doesn't
recognize).

Optional env vars (with their defaults): `IRC_SERVER`
(irc.chatnet.gg), `IRC_PORT` (6697), `HERALD_NICK` (Herald),
`HERALD_NICKSERV_ACCOUNT` (Herald), `HERALD_CHANNEL` (#CN-Bot),
`HERALD_HTTP_ADDR` (:8080).

`HERALD_REPO_CHANNELS` sends some repositories to more channels than the main
one, for example `owner/repo=#dev,#ops;owner/other=#dev`. Every repository
still announces in `HERALD_CHANNEL`; the extra channels are joined at start-up
and rejoined after a kick. Repository names are matched without regard to case.

## License

MIT, see [LICENSE](LICENSE).
