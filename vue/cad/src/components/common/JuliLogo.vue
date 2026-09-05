<script setup lang="ts">
withDefaults(defineProps<{ animated?: boolean }>(), { animated: false })
</script>

<template>
  <svg class="juli-logo" :class="{ 'is-drawing': animated }" viewBox="0 0 900 290" role="img" aria-label="巨力液压，JULI HYDRAULIC">
    <g v-if="animated" class="construction">
      <path d="M15 250H885 M15 35H885 M150 0V290 M290 0V290 M330 0V290 M865 0V290 M0 180H900" />
      <circle cx="150" cy="155" r="128" />
      <path d="M20 260H280 M20 253V267 M280 253V267 M300 30V245 M293 30H307 M293 245H307" />
    </g>
    <g class="brand">
      <path class="triangle" pathLength="1" d="M150 28L278 242H22Z" />
      <!-- 字母使用工程折线路径，避免依赖英文字体并可逐线绘制。 -->
      <path class="initials" pathLength="1" d="M92 131H116V167Q116 180 103 180Q90 180 90 166 M131 132L145 152L159 132 M145 152V180 M176 132V180H197" />
      <text class="inner-name" x="150" y="221" text-anchor="middle">巨液</text>
      <text class="company-name" x="330" y="154" textLength="525" lengthAdjust="spacingAndGlyphs">巨力液压</text>
      <text class="english-name" x="333" y="220" textLength="515" lengthAdjust="spacingAndGlyphs">JULI HYDRAULIC</text>
      <path v-if="animated" class="underline" pathLength="1" d="M330 243H860" />
    </g>
    <g v-if="animated" class="pen">
      <circle r="5" fill="#dffaff" />
      <circle r="12" fill="none" stroke="#65eaff" stroke-width="1" />
    </g>
  </svg>
</template>

<style scoped>
.juli-logo { width: 100%; display: block; overflow: visible; color: #348ee9; }
.brand { filter: drop-shadow(0 0 5px #0d60c333); }
.triangle { fill: none; stroke: currentColor; stroke-width: 11; stroke-linejoin: miter; }
.initials { fill: none; stroke: currentColor; stroke-width: 7; stroke-linecap: square; stroke-linejoin: round; }
text { fill: currentColor; font-family: 'Microsoft YaHei', 'Noto Sans SC', sans-serif; font-weight: 800; }
.inner-name { font-size: 31px; letter-spacing: 5px; }
.company-name { font-size: 100px; }
.english-name { font-family: Arial, sans-serif; font-size: 36px; font-weight: 700; }
.construction { stroke: #60d9f5; stroke-width: .65; fill: none; opacity: .18; animation: guides 4.8s ease both; }
.underline { fill: none; stroke: #61dfff; stroke-width: 1; }
.is-drawing .triangle { stroke-dasharray: 1; stroke-dashoffset: 1; animation: trace 1.6s cubic-bezier(.4,0,.2,1) .2s forwards; }
.is-drawing .initials { stroke-dasharray: 1; stroke-dashoffset: 1; animation: trace 1s ease 1s forwards; }
.is-drawing text { stroke: #60baff; stroke-width: 1; fill: transparent; stroke-dasharray: 600; stroke-dashoffset: 600; animation: lettering 2.2s ease 1.1s both; }
.is-drawing .company-name { animation-delay: 1.4s; }
.is-drawing .english-name { animation-delay: 1.8s; }
.is-drawing .underline { stroke-dasharray: 1; stroke-dashoffset: 1; animation: trace .8s ease 2.8s forwards; }
.is-drawing .brand { animation: ignite 5.4s ease both; transform-origin: center; }
.pen { offset-path: path('M150 28L278 242H22Z'); filter: drop-shadow(0 0 9px #4beaff); animation: pen-trace 1.6s cubic-bezier(.4,0,.2,1) .2s both; }
@keyframes trace { to { stroke-dashoffset: 0; } }
@keyframes lettering { 0% { stroke-dashoffset: 600; fill: transparent; } 65% { stroke-dashoffset: 0; fill: transparent; } 100% { stroke-dashoffset: 0; fill: #348ee9; } }
@keyframes ignite { 0%,55% { filter: drop-shadow(0 0 2px #198af533); } 74% { filter: drop-shadow(0 0 22px #38bbff) brightness(1.9); } 87% { filter: drop-shadow(0 0 9px #3897ff); transform: scale(1); opacity: 1; } 100% { filter: drop-shadow(0 0 28px #53e4ff); transform: scale(1.09); opacity: 0; } }
@keyframes pen-trace { 0% { offset-distance: 0%; opacity: 0; } 5% { opacity: 1; } 95% { opacity: 1; } 100% { offset-distance: 100%; opacity: 0; } }
@keyframes guides { 0% { opacity: 0; } 15%,60% { opacity: .18; } 100% { opacity: 0; } }
@media(prefers-reduced-motion:reduce) { .is-drawing * { animation: none !important; stroke-dashoffset: 0 !important; } .is-drawing text { fill: #348ee9; } .pen { display: none; } }
</style>
