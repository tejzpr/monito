import { writable } from 'svelte/store';

const messageStore = writable('');
const wsStatus = writable('');
var loc = window.location, new_uri;
if (loc.protocol === "https:") {
    new_uri = "wss:";
} else {
    new_uri = "ws:";
}
new_uri += "//" + loc.host;
new_uri += loc.pathname + "api/monitors/ws";

const socket = new WebSocket(new_uri);

socket.addEventListener('open', function (event) {
    wsStatus.set(true)
});

socket.addEventListener('close', function (event) {
    wsStatus.set(false)
});

socket.addEventListener('message', function (event) {
    messageStore.set(event.data);
});

const sendMessage = (message) => {
	if (socket.readyState <= 1) {
		socket.send(message);
	}
}

export default {
	subscribe: messageStore.subscribe,
    subscribeWSStatus: wsStatus.subscribe,
	sendMessage
}