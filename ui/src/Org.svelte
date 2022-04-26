<script>
    import { onMount } from 'svelte';
    import store from './store.js';
    $: org = {
        orgName: "",
        orgLogoURI: "",
        orgURI: ""
    }
    onMount(async () => {
        store.subscribeOrg(orgn => {
            org.orgName = orgn.orgName;
            org.orgLogoURI = orgn.orgLogoURI;
            org.orgURI = orgn.orgURI;
        });
    });
</script>

{#if org.orgName.length > 0}
<header class="p-3 bg-light text-white">
    <div class="container org">
        <div class="d-flex flex-wrap align-items-center justify-content-center justify-content-lg-start">
            <a href="{org.orgURI}" target="_blank" class="d-flex align-items-center mb-2 mb-lg-0 text-white text-decoration-none">
                <img src="{org.orgLogoURI}" class="orglogo" alt="{org.orgName}" title="{org.orgName}"/> 
                <span class="orglogo-text">{org.orgName}</span>
            </a>
        </div>
    </div>
</header>
{/if}

<style>
    .org {
        margin-top: 0px;
    }
    .org .orglogo-text{
        color: #000000;
        font-size: 2em;
        margin-left: 15px;
    }
    .org .orglogo {
        height: 2em;
    }
</style>