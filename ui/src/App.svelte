<script>
	import Header from './Header.svelte';
	import axios from "axios";
	import { onMount } from 'svelte';
	import store from './store.js';

	$: monitorData = {};
	$: notifyerror = null;
	const API_URL = "/api/";
	let monitors = [];

	function postError(err) {
		if (notifyerror == null) {
			notifyerror = err;
			setTimeout(() => {
				notifyerror = null;
			}, 5000);
		}
	}

	let selected = 'all';
	
	function onChange(event) {
		selected = event.currentTarget.value;
	}

	onMount(async () => {

		store.subscribe(currentMessage => {
			if (currentMessage.length > 0) {
				monitorData = JSON.parse(currentMessage);
			}
		})

		setInterval(() => {
	        store.sendMessage("all");
		}, 1000)
		

		try {
			const response = await axios.get(`${API_URL}monitors`);
			if (typeof response.data["monitors"] !== undefined) {
				monitors = response.data["monitors"];
			}
		} catch (error) {
			postError(error);
		}
	});
	
</script>


<main>
	<Header/>
	{#if notifyerror != null}
	<div class="alert alert-danger" role="alert">
		{notifyerror}
	</div>
	{/if}
	<div class="content">
		<div class="container body-main">
			<div class="form-check">
				<input class="form-check-input" type="radio" name="inlineRadioOptions" id="inlineRadio1" value="all" on:change={onChange} checked={selected==='all'}>
				<label class="form-check-label" for="inlineRadio1">View All Monitors</label>
			  </div>
			  <div class="form-check">
				<input class="form-check-input" type="radio" name="inlineRadioOptions" id="inlineRadio2" value="ok" on:change={onChange} checked={selected==='ok'}>
				<label class="form-check-label" for="inlineRadio2">View Monitors that have status OK </label>
			  </div>
			  <div class="form-check">
				<input class="form-check-input" type="radio" name="inlineRadioOptions" id="inlineRadio3" value="error" on:change={onChange} checked={selected==='error'}>
				<label class="form-check-label" for="inlineRadio3">View Monitors that have status ERROR</label>
			  </div>
		</div>
		<div class="container body-main">
			<ol class="list-group list-group-numbered">
				{#each monitors as monitor}
				
					{#if (typeof monitorData[monitor.name] === 'undefined' ? "Loading" : monitorData[monitor.name]["status"] === "OK") && (selected === 'ok' || selected === 'all')}
						<li class="list-group-item d-flex justify-content-between align-items-start">
							<div class="ms-2 me-auto">
								<div class="fw-bold">{monitor.name}</div>
								{monitor.description}
							</div>
							<span class="badge bg-success rounded-pill">OK</span>
						</li>
					{:else if (typeof monitorData[monitor.name] === 'undefined' ? "Loading" : monitorData[monitor.name]["status"] === "ERROR") && (selected === 'error' || selected === 'all')}
						<li class="list-group-item d-flex justify-content-between align-items-start">
							<div class="ms-2 me-auto">
								<div class="fw-bold">{monitor.name}</div>
								{monitor.description}
							</div>
							<span class="badge bg-danger rounded-pill">ERROR</span>
						</li>

					{/if}
				
				{/each}
			</ol>
		</div>
	</div>	
</main>


<style>
	.badge {
		width: 10em;
		height: 4em;
		color: #fff;
		display: inline-flex;
		align-items: center;
    	justify-content: center;
	}
	.body-main {
		margin-top:20px;
	}
</style>