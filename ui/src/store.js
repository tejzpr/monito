import { writable } from 'svelte/store';
import { toast } from '@zerodevx/svelte-toast';

const orgStore = writable({
    orgName: orgName,
    orgLogoURI: orgLogoURI,
    orgURI: orgURI,
});
  
const appMap = new Map([]);
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
            let msgMap = new Map(Object.entries(JSON.parse(event.data)))
            messageStore.set(msgMap);
            
            for (let [key, value] of msgMap) {
                if (appMap.has(key) && appMap.get(key) !== "INIT") {               
                    if (value.status === 'UP') {
                        toast.push(`${key} is ${value.status}`, {
                            pausable: true,
                            theme: {
                            '--toastBackground': '#48BB78',
                            '--toastBarBackground': '#2F855A'
                            }
                        })
                    } else if (value.status === 'DOWN') {
                        toast.push(`${key} is ${value.status}`, {
                            pausable: true,
                            theme: {
                            '--toastBackground': '#F56565',
                            '--toastBarBackground': '#A82E2E'
                            }
                        })
                    }
                }
                appMap.set(key, value.status);
            }
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