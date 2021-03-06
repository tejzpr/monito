<script>
	import Header from './Header.svelte';
	import Org from './Org.svelte';
	import axios from "axios";
	import moment from "moment";
	import { onMount } from 'svelte';
	import store from './store.js';
	import { SvelteToast } from '@zerodevx/svelte-toast'

	const toastOptions = { 
		initial: 0, 
		intro: { 
			y: -64 
		} 
	};

	$: monitorData = new Map();
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

	function isIterable(obj) {
		if (obj == null) {
			return false;
		}
		return typeof obj[Symbol.iterator] === 'function';
	}


	onMount(async () => {

		store.subscribe(currentMessage => {
			if (loading == true) {
				loading = false;
			}
			if (currentMessage.size > 0) {
				try {
					for (let [key, value] of currentMessage) {
						monitorData[key] = value;
					}
				} catch (err) {
					postError(err);
				}
			}
		})
		store.subscribeMessageSocketStatus(status => {
			if (status === true) {
				store.sendMessage("all");
			}
		})

		try {
			const response = await axios.get(`${API_URL}monitors`);
			if (typeof response.data["monitors"] !== undefined) {
				monitors = response.data["monitors"];
				if (monitors.length > 0) {
					monitors = monitors.map(monitor => {
						if (monitor.group !== "") {
							monitor.wGroup = monitor.group + " - " + monitor.name;
						} else {
							monitor.wGroup = monitor.name;
						}
						return monitor;
					});
					monitors.sort((a, b) => {return (a.wGroup > b.wGroup) ? 1 : ((b.wGroup > a.wGroup) ? -1 : 0);})
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
	<div class="wrap">
		<SvelteToast {toastOptions} />
	</div>
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
					<input class="form-check-input" type="radio" name="inlineRadioOptions" id="inlineRadio3" value="up" on:change={onChange} checked={selected==='up'}>
					<label class="form-check-label" for="inlineRadio3">View Services that have status <span class="badge bg-success rounded-pill">UP</span> </label>
				</div>
				<div class="form-check">
					<input class="form-check-input" type="radio" name="inlineRadioOptions" id="inlineRadio4" value="down" on:change={onChange} checked={selected==='down'}>
					<label class="form-check-label" for="inlineRadio4">View Services that have status <span class="badge bg-danger rounded-pill">DOWN</span></label>
				</div>
				<div class="form-check">
					<input class="form-check-input" type="radio" name="inlineRadioOptions" id="inlineRadio2" value="init" on:change={onChange} checked={selected==='init'}>
					<label class="form-check-label" for="inlineRadio2">View Services that have status <span class="badge bg-secondary rounded-pill">INIT</span> </label>
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
					
						{#if (typeof monitorData[monitor.name] !== 'undefined' && monitorData[monitor.name]["status"] === "UP") && (selected === 'up' || selected === 'all')}
							<li class="list-group-item d-flex justify-content-between align-items-start">
								<div class="ms-2 me-auto">
									<div class="fw-bold">{monitor.name}<span class="badge bg-info rounded-pill group-pill" title="Group - {monitor.group}">{monitor.group}</span></div>
									{monitor.description}
								</div>
								<span class="badge monitor bg-success rounded-pill" title="Status changed to UP {typeof monitorData[monitor.name]!== 'undefined'?moment(monitorData[monitor.name]["timestamp"]).fromNow() : ""}">UP</span>
							</li>
						{:else if (typeof monitorData[monitor.name] !== 'undefined' && monitorData[monitor.name]["status"] === "DOWN") && (selected === 'down' || selected === 'all')}
							<li class="list-group-item d-flex justify-content-between align-items-start">
								<div class="ms-2 me-auto">
									<div class="fw-bold">{monitor.name}<span class="badge bg-info rounded-pill group-pill" title="Group - {monitor.group}">{monitor.group}</span></div>
									{monitor.description}
								</div>
								<span class="badge monitor bg-danger rounded-pill" title="Status changed to DOWN {typeof monitorData[monitor.name] !== 'undefined'?moment(monitorData[monitor.name]["timestamp"]).fromNow() : ""}">DOWN</span>
							</li>
						{:else if (typeof monitorData[monitor.name] !== 'undefined' && monitorData[monitor.name]["status"] === "INIT") && (selected === 'init' || selected === 'all')}
							<li class="list-group-item d-flex justify-content-between align-items-start">
								<div class="ms-2 me-auto">
									<div class="fw-bold">{monitor.name}<span class="badge bg-info rounded-pill group-pill" title="Group - {monitor.group}">{monitor.group}</span></div>
									{monitor.description}
								</div>
								<span class="badge monitor bg-secondary rounded-pill" title="Waiting for status">INIT</span>
							</li>
						{/if}
					{/each}
				</ol>
			</div>
		</div>	
	{/if}
</main>


<style>
	.group-pill {
		margin-left:10px;
	}
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

	.wrap {
		--toastContainerTop: 0.5rem;
		--toastContainerRight: 0.5rem;
		--toastContainerBottom: auto;
		--toastContainerLeft: 0.5rem;
		--toastWidth: 100%;
		--toastMinHeight: 2rem;
		--toastPadding: 0 0.5rem;
		font-size: 0.875rem;
	}
	@media (min-width: 40rem) {
		.wrap {
		--toastContainerRight: auto;
		--toastContainerLeft: calc(50vw - 20rem);
		--toastWidth: 40rem;
		}
	}
</style>