const transport = new WebTransport('https://your-server.example/_webtransport');

await transport.ready; // resolves when connection established

// send a datagram (unreliable, low-latency)
const encoder = new TextEncoder();
transport.datagrams.send(encoder.encode('hello datagram'));

// receive datagrams
const reader = transport.datagrams.readable.getReader();
(async () => {
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    console.log('datagram received:', new TextDecoder().decode(value));
  }
})();