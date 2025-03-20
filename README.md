# websocketGO_task

IN TERMINAL:
# Install dependencies

# Run the server
go run main.go

# Open Chrome console for testing -
press F12 to open and open in incognito mode as there was some security issue in my system so open in incognito it will work

# Create connection 1 in one tab (incognito)
const ws1 = new WebSocket('ws://localhost:8080/ws');
ws1.onmessage = (event) => { console.log('Client 1 received:', event.data); };
ws1.onopen = () => { console.log('Client 1 connected'); };
   
# Create connection 2 in 2nd tab(incognito)
const ws2 = new WebSocket('ws://localhost:8080/ws');
ws2.onmessage = (event) => { console.log('Client 2 received:', event.data); };
ws2.onopen = () => { console.log('Client 2 connected'); };

# then open test.html in 3rd tab(incognito)
Click "Connect" on Client 1 and Client 2
Click "Send Message" on either client to test individual messages
Type something in the broadcast text field and click "Send to All"
Watch both clients receive the same broadcast message
Check terminal window to see logged messages with client IDs

