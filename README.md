# A Crappy Loadbalancer (written in GO)
This project is all about diving into Go and exploring its cool features like goroutines and channels. Let's get one thing straight: this isn't some top-tier, high-performance load balancer. It's clunky, it's basic, but hey, it works! Here's what it can handle without falling apart:

- Booting up servers
- Managing the state of the running servers
- Doing health checks and restarting unhealthy servers (but doesn’t bother queuing or redirecting requests during the restart—oops).
- Boots up additional servers based on load.
- Track basic metrics so you can watch your servers suffer
- Shout into the void with a message channel for runtime logs

Built this while learning Go's concurrency features, and it actually works... most of the time. It's rough around the edges, probably has more bugs than features, but hey – it load balances... sort of.
Consider this more of a "what I learned about Go" project rather than something you'd want anywhere near production.

## How to run?
1. Make sure you have golang installed in your OS.
2. Build the `/backend/main.go` into a binary. It will be used as the backend server and loadbalancer will execute this binary. 
3. In the `/loadbalancer` directory, edit the `config.json` file to your liking. There are two strategies: 'round-robing' & 'least-connection'.
4. Run the `main.go` inside the `/loadbalancer` directory.
