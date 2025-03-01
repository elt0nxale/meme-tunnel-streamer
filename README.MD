# meme-fetcher
- allows multiple client connections to *stream* real time memes from reddit's r/memes. 
- *stream* here refers to live [Server-Sent Events (SSE)](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events) coming through the http connection every 5 seconds
- [gorrilla/mux](https://pkg.go.dev/github.com/gorilla/mux#section-readme) is a multiplexer to handle serving multiple requests
- [ngrok](https://ngrok.com/docs/) is a reverse proxy allowing external clients to securely connect the local server

## Get Started
1. From the project root, run either
   
   - ```go run main.go```  (test locally, ngrok token not needed)
   - ```go run main.go --tunnel```  (expose local server to clients on a publicly accessible url, for e.g. your phones)
2.  Open either `localhost:8080` or your tunnel url in new tab
3.  ***Laugh*** at meme 
4.  Try with multiple tabs / windows
5.  ***Laugh*** at more memes 
   
## How it works
- We fetch and cache the top 26 memes from r/memes in memory every 5mins
- when a client connects to the server's main page, an http connection is opened serving a static `web/index.html`
- another special, long-lived http connection (Event Source) is opened and kept open while memes are streamed in realtime from the cache
- new clients connect, opening more connections to the Event Source (`/memes`) who each receive a unique sequence of memes from the shared cache 
- the drawer on the left *streams* the statuses of the connections, making them available to all 

### Credits
*_This project was intended to reinvent the wheel to learn Go better, inspired by https://www.youtube.com/watch?v=3qGxVYJF3IU&t=1172s and https://jprq.io_*