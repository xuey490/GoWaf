import{at as e,g as t,h as n,it as r,r as i,st as a,y as o}from"./resolve-slot-D0ESJhJD.js";import{n as s,t as c}from"./replaceable-CG1Qs4lR.js";import{S as l,St as u,V as d,b as f,f as p,h as m,p as h}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{t as g}from"./use-style-CXm9Jcwk.js";import{n as _,t as v}from"./icon-switch.cssr-o4i5UUsg.js";import{l as y}from"./Button-DUNxlS0h.js";var b=c(`clear`,()=>(()=>{let e=n(`c93f8499adf26ca3`);return e[0]||(e[0]=p(`svg`,{viewBox:`0 0 16 16`,version:`1.1`,xmlns:`http://www.w3.org/2000/svg`},[p(`g`,{stroke:`none`,"stroke-width":`1`,fill:`none`,"fill-rule":`evenodd`},[p(`g`,{fill:`currentColor`,"fill-rule":`nonzero`},[p(`path`,{d:`M8,2 C11.3137085,2 14,4.6862915 14,8 C14,11.3137085 11.3137085,14 8,14 C4.6862915,14 2,11.3137085 2,8 C2,4.6862915 4.6862915,2 8,2 Z M6.5343055,5.83859116 C6.33943736,5.70359511 6.07001296,5.72288026 5.89644661,5.89644661 L5.89644661,5.89644661 L5.83859116,5.9656945 C5.70359511,6.16056264 5.72288026,6.42998704 5.89644661,6.60355339 L5.89644661,6.60355339 L7.293,8 L5.89644661,9.39644661 L5.83859116,9.4656945 C5.70359511,9.66056264 5.72288026,9.92998704 5.89644661,10.1035534 L5.89644661,10.1035534 L5.9656945,10.1614088 C6.16056264,10.2964049 6.42998704,10.2771197 6.60355339,10.1035534 L6.60355339,10.1035534 L8,8.707 L9.39644661,10.1035534 L9.4656945,10.1614088 C9.66056264,10.2964049 9.92998704,10.2771197 10.1035534,10.1035534 L10.1035534,10.1035534 L10.1614088,10.0343055 C10.2964049,9.83943736 10.2771197,9.57001296 10.1035534,9.39644661 L10.1035534,9.39644661 L8.707,8 L10.1035534,6.60355339 L10.1614088,6.5343055 C10.2964049,6.33943736 10.2771197,6.07001296 10.1035534,5.89644661 L10.1035534,5.89644661 L10.0343055,5.83859116 C9.83943736,5.70359511 9.57001296,5.72288026 9.39644661,5.89644661 L9.39644661,5.89644661 L8,7.293 L6.60355339,5.89644661 Z`})])])],-1))})()),x=e(`base-clear`,`
 flex-shrink: 0;
 height: 1em;
 width: 1em;
 position: relative;
`,[r(`>`,[a(`clear`,`
 font-size: var(--n-clear-size);
 height: 1em;
 width: 1em;
 cursor: pointer;
 color: var(--n-clear-color);
 transition: color .3s var(--n-bezier);
 display: flex;
 `,[r(`&:hover`,`
 color: var(--n-clear-color-hover)!important;
 `),r(`&:active`,`
 color: var(--n-clear-color-pressed)!important;
 `)]),a(`placeholder`,`
 display: flex;
 `),a(`clear, placeholder`,`
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 `,[v({originalTransform:`translateX(-50%) translateY(-50%)`,left:`50%`,top:`50%`})])])]),S=[`onClick`,`onMousedown`],C=l({name:`BaseClear`,props:{clsPrefix:{type:String,required:!0},show:Boolean,onClear:Function},setup(e){return g(`-base-clear`,x,u(e,`clsPrefix`)),{handleMouseDown(e){e.preventDefault()}}},render(){let{clsPrefix:e}=this;return d(),m(`div`,{class:t(`${e}-base-clear`)},[f(_,null,{default:()=>this.show?(d(),m(`div`,{key:`dismiss`,class:t(`${e}-base-clear__clear`),onClick:this.onClear,onMousedown:this.handleMouseDown,"data-clear":!0},[o(()=>i(this.$slots.icon,()=>[(d(),h(s,{clsPrefix:e},{default:()=>(d(),h(b))},1032,[`clsPrefix`]))]))],42,S)):(d(),m(`div`,{key:`icon`,class:t(`${e}-base-clear__placeholder`)},[o(()=>this.$slots.placeholder?.())],2))},1024)],2)}}),w=l({name:`ChevronDown`,render(){return(()=>{let e=n(`ae90ecf811a811ac`);return e[0]||(e[0]=p(`svg`,{viewBox:`0 0 16 16`,fill:`none`,xmlns:`http://www.w3.org/2000/svg`},[p(`path`,{d:`M3.14645 5.64645C3.34171 5.45118 3.65829 5.45118 3.85355 5.64645L8 9.79289L12.1464 5.64645C12.3417 5.45118 12.6583 5.45118 12.8536 5.64645C13.0488 5.84171 13.0488 6.15829 12.8536 6.35355L8.35355 10.8536C8.15829 11.0488 7.84171 11.0488 7.64645 10.8536L3.14645 6.35355C2.95118 6.15829 2.95118 5.84171 3.14645 5.64645Z`,fill:`currentColor`})],-1))})()}}),T=l({name:`InternalSelectionSuffix`,props:{clsPrefix:{type:String,required:!0},showArrow:{type:Boolean,default:void 0},showClear:{type:Boolean,default:void 0},loading:Boolean,onClear:Function},setup(e,{slots:n}){return()=>{let{clsPrefix:r}=e;return d(),h(y,{clsPrefix:r,class:t(`${r}-base-suffix`),strokeWidth:24,scale:.85,show:e.loading},{default:()=>e.showArrow?(d(),h(C,{key:1,clsPrefix:r,show:e.showClear,onClear:e.onClear},{placeholder:()=>(d(),h(s,{clsPrefix:r,class:t(`${r}-base-suffix__arrow`)},{default:()=>i(n.default,()=>[(d(),h(w))])},1032,[`clsPrefix`,`class`]))},1032,[`clsPrefix`,`show`,`onClear`])):null},1032,[`clsPrefix`,`class`,`show`])}}});export{w as n,C as r,T as t};