<script>
	import Header from './Header.svelte';
	import Org from './Org.svelte';
	import axios from "axios";
	import moment from "moment";
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
	let loading = true;
	function onChange(event) {
		selected = event.currentTarget.value;
	}


	onMount(async () => {

		store.subscribe(currentMessage => {
			if (currentMessage.length > 0) {
				if (loading == true) {
					loading = false;
				}
				try {
					monitorData = JSON.parse(currentMessage);
				} catch (err) {
					postError(err);
				}
				
			}
		})

		setInterval(() => {
	        store.sendMessage("all");
		}, 5000)
		

		try {
			const response = await axios.get(`${API_URL}monitors`);
			if (typeof response.data["monitors"] !== undefined) {
				monitors = response.data["monitors"];
				if (monitors.length > 0) {
					monitors = monitors.map(monitor => {
						if (monitor.group !== "") {
							monitor.name = monitor.group + " - " + monitor.name;
						}
						return monitor;
					});
					monitors.sort((a, b) => {return (a.name > b.name) ? 1 : ((b.name > a.name) ? -1 : 0);})
				}
			}
		} catch (error) {
			postError(error);
		}
	});
	
</script>


<main>
	<Header/>
	<Org/>
	{#if notifyerror != null}
	<div class="alert alert-danger" role="alert">
		{notifyerror}
	</div>
	{/if}
	{#if loading == true}
		<div class="content">
			<div class="container-fluid body-main">
				<div class="row flex-xl-nowrap">
					<div class="col-12">
						<div class="d-flex justify-content-center">
							<div class="spinner-border" role="status">
								<span class="sr-only"></span>
							</div>
						</div>
						<div class="d-flex loading justify-content-center">
							Initializing, please wait...
						</div>	
					</div>
				</div>
			</div>
		</div>	
	{/if}
	{#if loading == false}
		<div class="content">
			<div class="container body-main">
				<div class="form-check">
					<input class="form-check-input" type="radio" name="inlineRadioOptions" id="inlineRadio1" value="all" on:change={onChange} checked={selected==='all'}>
					<label class="form-check-label" for="inlineRadio1">View All Services</label>
				</div>
				<div class="form-check">
					<input class="form-check-input" type="radio" name="inlineRadioOptions" id="inlineRadio2" value="ok" on:change={onChange} checked={selected==='ok'}>
					<label class="form-check-label" for="inlineRadio2">View Services that have status <span class="badge bg-success rounded-pill">UP</span> </label>
				</div>
				<div class="form-check">
					<input class="form-check-input" type="radio" name="inlineRadioOptions" id="inlineRadio3" value="error" on:change={onChange} checked={selected==='error'}>
					<label class="form-check-label" for="inlineRadio3">View Services that have status <span class="badge bg-danger rounded-pill">DOWN</span></label>
				</div>
			</div>
			<div class="container body-main">
				<ol class="list-group">
							<li class="list-group-item d-flex justify-content-between align-items-start header">
								<div class="ms-2 me-auto">
									<div class="fw-bold">Services</div>
								</div>
								<span class="fw-bold">Status</span>
							</li>
					{#each monitors as monitor}
					
						{#if (typeof monitorData[monitor.name] === 'undefined' ? "Loading" : monitorData[monitor.name]["status"] === "OK") && (selected === 'ok' || selected === 'all')}
							<li class="list-group-item d-flex justify-content-between align-items-start">
								<div class="ms-2 me-auto">
									<div class="fw-bold">{monitor.name}</div>
									{monitor.description}
								</div>
								<span class="badge monitor bg-success rounded-pill" title="Status changed to UP {typeof monitorData[monitor.name]!== 'undefined'?moment(monitorData[monitor.name]["timestamp"]).fromNow() : ""}">UP</span>
							</li>
						{:else if (typeof monitorData[monitor.name] === 'undefined' ? "Loading" : monitorData[monitor.name]["status"] === "ERROR") && (selected === 'error' || selected === 'all')}
							<li class="list-group-item d-flex justify-content-between align-items-start">
								<div class="ms-2 me-auto">
									<div class="fw-bold">{monitor.name}</div>
									{monitor.description}
								</div>
								<span class="badge monitor bg-danger rounded-pill" title="Status changed to DOWN {typeof monitorData[monitor.name] !== 'undefined'?moment(monitorData[monitor.name]["timestamp"]).fromNow() : ""}">DOWN</span>
							</li>
						{/if}
					{/each}
				</ol>
			</div>
		</div>	
	{/if}
</main>


<style>
	.badge.monitor {
		width: 4em;
		height: 4em;
		color: #fff;
		display: inline-flex;
		align-items: center;
    	justify-content: center;
	}
	.loading {
		margin-top:20px;
	}
	.body-main {
		margin-top:20px;
	}
	.list-group li {
		border-left-width: 0px;
		border-right-width: 0px;
	}
	.list-group .header {
		border-top-width: 0px;
		margin-top: 20px;
	}
	.form-check-label, .form-check-input {
		cursor: pointer;
	}
</style>