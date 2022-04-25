<script>
    import { onMount } from 'svelte';
    import store from './store.js';
    $: status = false
    onMount(async () => {
        store.subscribeWSStatus(st => {
            status = st;
        });
    });
</script>


<header class="p-3 bg-dark text-white">
    <div class="container-fluid">
        <div class="d-flex flex-wrap align-items-center justify-content-center justify-content-lg-start">
        <a href="/" class="d-flex align-items-center mb-2 mb-lg-0 text-white text-decoration-none">
            <img src="/static/images/logo.png" width="40" height="32" alt="Monito" title="Monito"/> 
            <span class="logo-text">Monito</span>
        </a>
        {#if status === true}
            <span class="ms-auto badge online ml-2">Online</span>
        {:else if status === false}
            <a href="/" class="ms-auto badge offline ml-2">Offline (Refresh)</a>
        {/if}
        </div>
    </div>
</header>

<style>
    .logo-text{
        margin-left:15px;
    }
    .online{
        background-color: #28a745;
    }
    .offline{
        background-color: #dc3545;
        color: #ffffff;
    }
    a.offline:hover, a.offline{
        text-decoration: none;
    }
    
</style>