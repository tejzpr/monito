import { writable } from 'svelte/store';

const orgStore = writable({
    orgName: orgName,
    orgLogoURI: orgLogoURI,
    orgURI: orgURI,
});
const messageStore = writable(new Map());
const wsStatus = writable('');
const messageSocketStatus = writable('');
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
    wsStatus.set(true);
    messageSocketStatus.set(true);
});

socket.addEventListener('close', function (event) {
    wsStatus.set(false);
});

socket.addEventListener('message', function (event) {
    try {
        if (event.data.length > 0) {
            messageStore.set(new Map(Object.entries(JSON.parse(event.data))));
        }
    } catch (e) {
        console.log(e);
    }
});

const sendMessage = (message) => {
	if (socket.readyState <= 1) {
		socket.send(message);
	}
}

export default {
	subscribe: messageStore.subscribe,
    subscribeMessageSocketStatus: messageSocketStatus.subscribe,
    subscribeWSStatus: wsStatus.subscribe,
    subscribeOrg: orgStore.subscribe,
	sendMessage
}