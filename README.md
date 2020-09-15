# README

This signaling service provides facilities for establishing bidirectional communication between WebRTC peers. It aims to be as simple as possible, delegating most state and identity management to clients.

It does:
1. Associate peers by using a rudimentary "room" concept built around a distributed counting semaphore.
1. Provide clients with "an anonymous peer is available" events.
1. Provide clients with a transient session ID specific to the signaling service to allow clients with unreliable connections to rejoin their "room".
1. Leverage a single Redis instance for all locking and messaging operations.

It does not:
1. Do auth of any kind.
1. Provide clients with "a peer disappeared" events.
1. Attempt to prevent "room hijacking" beyond using a configurable timeout before new clients can replace dead ones.

In the future, it could:
1. Leverage auth provided by an external mechanism.
1. Scale by delegating locking and messaging to separate Redis deployments, or different platforms entirely.

## Service Endpoints

The service exposes it's endpoints via HTTP/1.x on the port specified by the `--addr` flag (default `:8080`).

### Events

Events are JSON messages in the following format:

```json
{
    "kind": "ENUM",
    "body": "STRING"
}
```

`kind` is a string enumeration that accepts one of these values:
 * `PEER`: Indicates that a peer is available.
 * `OFFER`: Indicates that the event body is an SDP offer.
 * `ANSWER`:  Indicates that the event body is an SDP answer.

`body` is an arbitrary string that should be interpreted based on the value of `kind`.

### `GET /call/{call}`

Join the room identified by `{call}` or renew the lease on a seat in the room. Clients should periodically call this endpoint to renew their lease, at an interval smaller than the value of `--seat-max-age` (default `30s`).

* Returns `200` on successful room join/renew.
* Returns `409` when attempting to join a room that is already full.

### `POST /call/{call}/signal`

Publishing peering data to other clients in room `{call}`. Data is not persistent; clients will only see data published after they've joined the room.

* Returns `200` on a successful publish (this does not guarantee that any other clients have received the published data).
* Returns `409` when attempting to publish to a room that the client isn't a member of.

### `GET /call/{call}/ws`

Establishes a websocket connection for receiving published peering data as events (see above). Returns a `409` when attempting to subscribe to the data feed for a room that the client isn't a member of.

## Infrastructure Endpoints

Infrastructure endpoints are served on the port specified by the `--infra-addr` flag (default `:8090`).

* `/debug`: Pprof endpoint
* `/health`: Service healthcheck endpoint; returns `200` on a healthy service.
