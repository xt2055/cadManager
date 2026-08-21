<script setup lang="ts">
defineOptions({
  name: 'DesktopStatusBar',
})

</script>

<template>
  <footer id="statusbar">
    <span><span class="status-dot"></span>服务状态 · 待连接</span>
    <span>转换队列 <b class="status-ok">—</b></span>
    <span>在线用户 —</span>
    <span class="sp"></span>
    <span class="zoom-ind">缩放 100%</span>
    <span>当前用户 · 未登录</span>
    <span>v0.1.0</span>
  </footer>
</template>
