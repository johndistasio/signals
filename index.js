"use strict";

let state = {
  call: "",
  session: "",
  ws: null,
};

const id = (i) => document.getElementById(i);

const get = (key) => window.sessionStorage.getItem(key);

const set = (key, value) => window.sessionStorage.setItem(key, value);

const newCallId = () => `${Math.floor(Math.random() * 1_000)}-${Math.floor(Math.random() * 1_000)}`;

window.addEventListener("load", (e) => {
  const callInput = document.getElementById("callInput");
  const messageInput = document.getElementById("messageInput");

  const connectButton = document.getElementById("connectButton");
  const sendButton = document.getElementById("sendButton");

  callInput.value = newCallId();

  callInput.addEventListener("input", (e) => {
    if (callInput.value === "") {
      connectButton.disabled = true;
    } else {
      connectButton.disabled = false;
    }
  });

  messageInput.addEventListener("input", (e) => {
    if (messageInput.value === "") {
      sendButton.disabled = true;
    } else {
      sendButton.disabled = false;
    }
  });
});

function display(msg) {
  const display = document.getElementById("display");
  display.value = display.value + msg + "\n";
}

function connect() {
  const callInput = document.getElementById("callInput");
  const connectButton = document.getElementById("connectButton");

  const call = callInput.value;

  fetch(`http://localhost:8080/call/${call}`)
    .then(response => {
      if (!response.ok) {
        if (response.status === 409) {
          throw Error(`No available seats in call "${call}".`);
        } else {
          throw Error(`HTTP ${response.status} on call join.`);
        }
      }
      return response;
    })
    .then(response => response.json())
    .then((json) => {
      display(JSON.stringify(json));

      const ws = new WebSocket("ws://localhost:8080/ws");

      ws.onopen = (e) => {
        let handshake = {
          kind: "JOIN",
          call: call,
          session: json["session"],
        };

        console.log(handshake);
        ws.send(JSON.stringify(handshake));
      };

      ws.onmessage = (e) => {
        display(e.data);
      };

      ws.onclose = (e) => {
        if (e.reason) {
          display(`WebSocket Closed: ${e.reason}`);
        } else {
          display("WebSocket Closed");
        }
      };

      state.call = call;
      state.session = json["session"];
      state.ws = ws;
    })

  connectButton.disabled = true;
}

function send() {
  const messageInput = id("messageInput");

  if (messageInput.value === "") {
    return;
  }

  let signal = {
    kind: "OFFER",
    body: messageInput.value,
    call: state.call,
    session: state.session,
  }

  console.log(signal);

  fetch("http://localhost:8080/signal", {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(signal),
  })
}
