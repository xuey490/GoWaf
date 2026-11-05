import{r as e,t}from"./Tabs-B9fBMyNB.js";import{Et as n,Gt as r,S as i,V as a,_t as o,at as s,b as c,d as l,f as u,h as d,ht as f,y as p}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{n as m}from"./vue-i18n-D4LRfH1J.js";import{t as h}from"./Tag-CfwO544n.js";import{t as g}from"./Card-D_p3JLbh.js";import{t as _}from"./_plugin-vue_export-helper-BDNMzG2s.js";import{t as v}from"./art-table-uf51iuES.js";var y=`{
  "name": "Fssadmin(naive-ui版)",
  "version": "1.0.0",
  "type": "module",
  "engines": {
    "node": ">=22.12.0",
    "pnpm": ">=8.8.0"
  },
  "scripts": {
    "dev": "vite --open",
    "build": "node scripts/check-i18n.mjs && vue-tsc --noEmit && vite build",
    "check:i18n": "node scripts/check-i18n.mjs",
    "serve": "vite preview",
    "lint": "eslint",
    "fix": "eslint --fix",
    "lint:prettier": "prettier --write \\"**/*.{js,cjs,ts,json,tsx,css,less,scss,vue,html,md}\\"",
    "lint:stylelint": "stylelint  \\"**/*.{css,scss,vue}\\" --fix",
    "lint:lint-staged": "lint-staged",
    "prepare": "husky",
    "commit": "git-cz",
    "clean:dev": "tsx scripts/clean-dev.ts"
  },
  "config": {
    "commitizen": {
      "path": "node_modules/cz-git"
    }
  },
  "lint-staged": {
    "*.{js,ts,mjs,mts,tsx}": [
      "eslint --fix",
      "prettier --write"
    ],
    "*.{cjs,json,jsonc}": [
      "prettier --write"
    ],
    "*.vue": [
      "eslint --fix",
      "stylelint --fix --allow-empty-input",
      "prettier --write"
    ],
    "*.{html,htm}": [
      "prettier --write"
    ],
    "*.{scss,css,less}": [
      "stylelint --fix --allow-empty-input",
      "prettier --write"
    ],
    "*.{md,mdx}": [
      "prettier --write"
    ],
    "*.{yaml,yml}": [
      "prettier --write"
    ]
  },
  "dependencies": {
    "naive-ui": "^2.45.3",
    "@iconify/icons-ri": "^1.2.10",
    "@iconify/json": "^2.2.546",
    "@iconify/vue": "^5.0.4",
    "@tailwindcss/vite": "^4.3.3",
    "@vicons/antd": "^0.13.0",
    "@vue/reactivity": "^3.5.44",
    "@vueuse/core": "^15.0.0",
    "@wangeditor/editor": "^5.1.23",
    "@wangeditor/editor-for-vue": "next",
    "axios": "^1.20.0",
    "crypto-js": "^4.2.0",
    "dayjs": "^1.11.23",
    "echarts": "^6.1.0",
    "file-saver": "^2.0.5",
    "highlight.js": "^11.12.0",
    "md-editor-v3": "^7.1.0",
    "mitt": "^3.0.1",
    "nprogress": "^0.2.0",
    "ohash": "^2.0.12",
    "pinia": "^4.1.0",
    "pinia-plugin-persistedstate": "^4.7.1",
    "qrcode.vue": "^3.11.0",
    "spark-md5": "^3.0.2",
    "tailwindcss": "^4.3.3",
    "vue": "^3.5.44",
    "vue-draggable-plus": "^0.6.1",
    "vue-i18n": "^11.4.16",
    "vue-router": "^5.4.0",
    "xgplayer": "^3.0.26",
    "xlsx": "^0.18.5"
  },
  "devDependencies": {
    "@commitlint/cli": "^21.2.4",
    "@commitlint/config-conventional": "^21.2.4",
    "@eslint/js": "^10.0.1",
    "@types/node": "^26.6.5",
    "@types/spark-md5": "^3.0.5",
    "@typescript-eslint/eslint-plugin": "^8.72.0",
    "@typescript-eslint/parser": "^8.72.0",
    "@vitejs/plugin-vue": "^6.0.10",
    "@vue/compiler-sfc": "^3.5.44",
    "commitizen": "^4.3.2",
    "cz-git": "^1.14.0",
    "eslint": "^10.13.0",
    "eslint-config-prettier": "^10.1.8",
    "eslint-plugin-prettier": "^5.5.7",
    "eslint-plugin-vue": "^10.11.1",
    "globals": "^17.13.0",
    "husky": "^9.1.7",
    "lint-staged": "^17.6.0",
    "prettier": "^3.9.10",
    "sass": "^1.106.1",
    "stylelint": "^17.16.0",
    "stylelint-config-html": "^2.0.0",
    "stylelint-config-recess-order": "^7.8.2",
    "stylelint-config-recommended-scss": "^17.0.1",
    "stylelint-config-recommended-vue": "^2.0.0",
    "stylelint-config-standard": "^40.0.0",
    "tsx": "^4.23.15",
    "typescript": "^6.0.3",
    "typescript-eslint": "^8.72.0",
    "unplugin-auto-import": "^21.1.0",
    "unplugin-vue-components": "^32.1.0",
    "vite": "^8.3.5",
    "vite-plugin-compression2": "^2.5.3",
    "vite-plugin-vue-devtools": "^8.2.1",
    "vue-demi": "^0.14.10",
    "vue-img-cutter": "^3.1.1",
    "vue-tsc": "^3.3.13"
  }
}
`,b={class:`deps-page art-full-height`},x={class:`flex-cb gap-3`},S={class:`m-0`},C={class:`flex items-center gap-2`},w={class:`deps-tab-label`},T={class:`deps-tab-label`},E=10,D=_(i({name:`Dependencies`,__name:`index`,setup(i){let{t:_}=m(),D=JSON.parse(y),O=D.name??`-`,k=D.version??`0.0.0`,A=e=>Object.entries(e??{}).map(([e,t])=>({name:e,version:t})).sort((e,t)=>e.name.localeCompare(t.name)),j={dependencies:A(D.dependencies),devDependencies:A(D.devDependencies)},M=o(`dependencies`),N=f({dependencies:{current:1,size:E},devDependencies:{current:1,size:E}}),P=e=>{let{current:t,size:n}=N[e];return j[e].slice((t-1)*n,t*n)},F=e=>({current:N[e].current,size:N[e].size,total:j[e].length}),I=(e,t)=>{N[e].size=t,N[e].current=1},L=(e,t)=>{N[e].current=t},R=l(()=>[{prop:`name`,label:_(`deps.columns.name`),minWidth:260,ellipsis:!0},{prop:`version`,label:_(`deps.columns.version`),width:180}]);return(i,o)=>{let l=h,f=v,m=e,y=t,E=g;return a(),d(`div`,b,[c(E,{class:`art-table-card`},{header:s(()=>[u(`div`,x,[u(`h4`,S,r(n(_)(`deps.title`)),1),u(`div`,C,[c(l,{type:`info`,size:`small`},{default:s(()=>[p(r(n(O)),1)]),_:1}),c(l,{size:`small`},{default:s(()=>[p(`v`+r(n(k)),1)]),_:1})])])]),default:s(()=>[c(y,{value:M.value,"onUpdate:value":o[4]||(o[4]=e=>M.value=e),type:`line`,class:`deps-tabs`,animated:``},{default:s(()=>[c(m,{name:`dependencies`},{tab:s(()=>[u(`span`,w,[p(r(n(_)(`deps.dependencies.title`))+` `,1),c(l,{size:`small`,type:`info`,round:``},{default:s(()=>[p(r(j.dependencies.length),1)]),_:1})])]),default:s(()=>[c(f,{"row-key":`name`,data:P(`dependencies`),columns:R.value,pagination:F(`dependencies`),"empty-text":n(_)(`deps.empty`),"onPagination:sizeChange":o[0]||(o[0]=e=>I(`dependencies`,e)),"onPagination:currentChange":o[1]||(o[1]=e=>L(`dependencies`,e))},null,8,[`data`,`columns`,`pagination`,`empty-text`])]),_:1}),c(m,{name:`devDependencies`},{tab:s(()=>[u(`span`,T,[p(r(n(_)(`deps.devDependencies.title`))+` `,1),c(l,{size:`small`,type:`warning`,round:``},{default:s(()=>[p(r(j.devDependencies.length),1)]),_:1})])]),default:s(()=>[c(f,{"row-key":`name`,data:P(`devDependencies`),columns:R.value,pagination:F(`devDependencies`),"empty-text":n(_)(`deps.empty`),"onPagination:sizeChange":o[2]||(o[2]=e=>I(`devDependencies`,e)),"onPagination:currentChange":o[3]||(o[3]=e=>L(`devDependencies`,e))},null,8,[`data`,`columns`,`pagination`,`empty-text`])]),_:1})]),_:1},8,[`value`])]),_:1})])}}}),[[`__scopeId`,`data-v-e8e2a526`]]);export{D as default};