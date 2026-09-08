<script setup>
import TopBar from '../components/TopBar.vue';
import SideBar from '../components/SideBar.vue';
</script>

<template>
    <div class="app">
        <TopBar />
        <div class="app-body">
            <SideBar />
            <main class="content">
                <router-view v-slot="{ Component }">
                    <transition name="fade-switch" mode="out-in">
                        <component :is="Component" />
                    </transition>
                </router-view>
            </main>
        </div>
    </div>
</template>

<style scoped>
.app {
    min-height: 100vh;
}

.app-body {
    display: flex;
    min-height: 100vh;
}

.content {
    flex: 1;
    min-width: 0;
    margin-left: 232px;
    padding-top: 52px;
}
</style>
