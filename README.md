# README

This signaling service provides facilities for establishing bidirectional communication between WebRTC peers. It aims to be as simple as possible, delegating most state and identity management to clients.

It does:
1. Associate peers by using a rudimentary "room" concept built around a distributed counting semaphore.
1. Provide clients with "a peer is available" events.
1. Provide clients with a transient session ID specific to the signaling service to allow clients with unreliable connections to rejoin their "room".
1. Leverage a single Redis instance for all locking and messaging operations.

It does not:
1. Do auth of any kind.
1. Provide clients with "a peer disappeared" events.
1. Attempt to prevent "room hijacking" beyond using a configurable timeout before new clients can replace dead ones.

In the future, it could:
1. Leverage auth provided by an external mechanism.
1. Scale by delegating locking and messaging to separate Redis deployments, or different platforms entirely.

## Signaling

TODO describe the intended request flow at a high level

### Endpoints

The service exposes it's endpoints via HTTP/1.x on the port specified by the `--addr` flag or `SIGNALS_ADDR` environment variable (default `:8080`).

#### Message Format

Messages exchanged with the service use a JSON format:

```json
{
  "body": STRING,
  "call": STRING,
  "kind": STRING,
  "session": STRING,
}
```

* `body` is an arbitrary string that is interpreted based upon the value of `kind`.
* `call` 
* `kind` is an enumeration that with endpoint-specific values.
* `session` tbd

#### `GET /call/{call}`

Joins the call identified by `{call}`. On a successful join, this endpoint returns a JSON message used to connect
to the `/ws` endpoint for peering updates:

```json
{
  "kind": "JOIN",
  "call": "<call>",
  "session": "<session>"
}
```

Where `<call>` is the call that was joined and `<session>` is a call membership token.

Return Codes:

* `200`: Successful call join.
* `409`: No seats are available for the specified call.

#### `POST /signal`

Publishes peering data to other clients in a room. Data is not persistent; clients will only see data published after 
they've joined the room. Published data must adhere to this format:

```json
{
  "kind": "OFFER|ANSWER",
  "call": "<call>",
  "session": "<session>",
  "body": "<body>"
}
```

Return codes:

* `200`: Successful publish (this does not guarantee that any other clients have received the published data).
* `409`: Attempted to publish to a room that the client isn't a member of.

#### `GET /ws`

Establishes a websocket connection for receiving peering data and maintaining presence within a room.

##### Handshake

todo

##### Presence

todo

##### Peering Data

todo

## Healthcheck & Debug Endpoints

These endpoints are served on the port specified by the `--healthz-addr` flag or `SIGNALS_HEALTHZ_ADDR` environment variable (default `:8090`).

* `/debug`: Pprof endpoint
* `/healthz`: Service healthcheck endpoint; returns `200` on a healthy service.
