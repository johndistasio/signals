# README

This signaling service provides facilities for establishing bidirectional communication between WebRTC peers. It aims to be as simple as possible, delegating most state and identity management to clients.

It does:
1. Associate peers by using a rudimentary "room" concept built around a distributed counting semaphore.
1. Provide clients with "a peer is available" events.
1. Provide clients with a transient session ID specific to the signaling service to allow clients with unreliable connections to rejoin their "room".
1. Leverage a single Redis instance for all locking and messaging operations.

It does not:
1. Provide clients with "a peer disappeared" events.
1. Provide clients with a way to uniquely identify peers beyond anything peers provide themselves.
1. Attempt to prevent "room hijacking" beyond using a configurable timeout before new clients can replace dead ones.

## Signaling

The high-level client flow for signaling is:

1. Join a call with `GET /call/{call}`; then
1. Open a websocket connection with `/ws` for establishing presence and receiving peering data; then
1. Send peering data with `POST /signal`.

### Endpoints

The service exposes it's endpoints via HTTP/1.x on the port specified by the `--addr` flag or `SIGNALS_ADDR` environment variable (default `:8080`).

#### Message Format

Messages exchanged with the service use a JSON format:

```json
{
  "kind": "string",
  "call": "string",
  "session": "string",
  "body": "string"
}
```

* `kind` is an enumeration that with endpoint-specific values.
* `call` is the name of the call.
* `session` is a session ID for the call.
* `body` is an arbitrary string that is interpreted based upon the value of `kind`.

#### `GET /call/{call}`

Joins the call identified by `{call}`. On a successful join, this endpoint returns a handshake message used to connect
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

#### `GET /ws`

Establishes a websocket connection for receiving peering data and maintaining presence within a room. After a successful
handshake, the server sends peering data to clients through the websocket. The server ignores messages sent by the client.

##### Handshake

Establishing a websocket connection requires an application-level handshake. A client must forward the handshake message
received from the `GET /call/{call}` endpoint immediately after opening a websocket connection. Upon receipt of a valid
handshake message, the websocket server will respond with:

```json
{
  "kind": "WELCOME",
  "call": "<call>"
}
```

The websocket server will then forward peering events to the client.

The server will close the websocket connection during the handshake phase if:

* The client does not send the handshake before the configured timeout.
* The handshake message is malformed.
* The client's session has expired.

##### Presence

The websocket connection maintains the client's presence on a call. The server will send a websocket `PING` on a
configurable interval. If the client responds with a `PONG` then the server will renew their session for the call.

The server will close the connection if the client's session cannot be renewed for any reason.

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

## Healthcheck & Debug Endpoints

These endpoints are served on the port specified by the `--healthz-addr` flag or `SIGNALS_HEALTHZ_ADDR` environment variable
(default `:8090`).

* `/debug`: Pprof endpoint
* `/healthz`: Service healthcheck endpoint; returns `200` on a healthy service.
