// hls.js 只发布 dist/hls.light.mjs，没有配套的 .d.ts（只有 hls.d.ts / hls.d.mts），
// 在 moduleResolution: "bundler" 下从 'hls.js/light' 导入会缺类型。
// light 构建的对外形状与主构建一致，因此直接复用主构建的类型声明。
declare module 'hls.js/light' {
  export * from 'hls.js';
  export { default } from 'hls.js';
}
