import{at as e,c as t,ct as n,g as r,it as i,l as a,ut as o,w as s,y as c}from"./resolve-slot-D0ESJhJD.js";import{ut as l}from"./user-ZPntuAC8.js";import{o as u}from"./replaceable-CG1Qs4lR.js";import{a as d}from"./Scrollbar-D9NqNphl.js";import{S as f,V as p,Wt as m,_t as h,b as g,d as _,f as v,h as y,it as b,p as x}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{t as S}from"./runtime-dom.esm-bundler-B3Pl3ttw.js";import{t as C}from"./use-compitable-CgN8vuBj.js";import{l as w,u as T}from"./Button-DUNxlS0h.js";var E=i([i(`@keyframes spin-rotate`,`
 from {
 transform: rotate(0);
 }
 to {
 transform: rotate(360deg);
 }
 `),e(`spin-container`,`
 position: relative;
 `,[e(`spin-body`,`
 position: absolute;
 top: 50%;
 left: 50%;
 transform: translateX(-50%) translateY(-50%);
 `,[d()])]),e(`spin-body`,`
 display: inline-flex;
 align-items: center;
 justify-content: center;
 flex-direction: column;
 `),e(`spin`,`
 display: inline-flex;
 height: var(--n-size);
 width: var(--n-size);
 font-size: var(--n-size);
 color: var(--n-color);
 `,[n(`rotate`,`
 animation: spin-rotate 2s linear infinite;
 `)]),e(`spin-description`,`
 display: inline-block;
 font-size: var(--n-font-size);
 color: var(--n-text-color);
 transition: color .3s var(--n-bezier);
 margin-top: 8px;
 `),e(`spin-content`,`
 opacity: 1;
 transition: opacity .3s var(--n-bezier);
 pointer-events: all;
 `,[n(`spinning`,`
 user-select: none;
 -webkit-user-select: none;
 pointer-events: none;
 opacity: var(--n-opacity-spinning);
 `)])]),D={small:20,medium:18,large:16},O={...t.props,contentClass:String,contentStyle:[Object,String],description:String,size:{type:[String,Number],default:`medium`},show:{type:Boolean,default:!0},rotate:{type:Boolean,default:!0},spinning:{type:Boolean,validator:()=>!0,default:void 0},delay:Number,...T,strokeWidth:Number},k=f({name:`Spin`,props:O,slots:Object,setup(e){let{mergedClsPrefixRef:n,inlineThemeDisabled:r}=s(e),i=t(`Spin`,`-spin`,E,l,e,n),c=_(()=>{let{size:t}=e,{common:{cubicBezierEaseInOut:n},self:r}=i.value,{opacitySpinning:a,color:s,textColor:c}=r;return{"--n-bezier":n,"--n-opacity-spinning":a,"--n-size":typeof t==`number`?u(t):r[o(`size`,t)],"--n-color":s,"--n-text-color":c}}),d=r?a(`spin`,_(()=>{let{size:t}=e;return typeof t==`number`?String(t):t[0]}),c,e):void 0,f=C(e,[`spinning`,`show`]),p=h(!1);return b(t=>{let n;if(f.value){let{delay:r}=e;if(r){n=window.setTimeout(()=>{p.value=!0},r),t(()=>{clearTimeout(n)});return}}p.value=f.value}),{mergedClsPrefix:n,active:p,mergedStrokeWidth:_(()=>{let{strokeWidth:t}=e;if(t!==void 0)return t;let{size:n}=e;return D[typeof n==`number`?`medium`:n]}),cssVars:r?void 0:c,themeClass:d?.themeClass,onRender:d?.onRender}},render(){let{$slots:e,mergedClsPrefix:t,description:n}=this,i=e.icon&&this.rotate,a=(n||e.description)&&(p(),y(`div`,{class:r(`${t}-spin-description`)},[c(()=>n||e.description?.())],2)),o=e.icon?(p(),y(`div`,{key:1,class:r([`${t}-spin-body`,this.themeClass])},[v(`div`,{class:r([`${t}-spin`,i&&`${t}-spin--rotate`]),style:m(e.default?``:this.cssVars)},[c(()=>e.icon())],6),c(()=>a)],2)):(p(),y(`div`,{key:2,class:r([`${t}-spin-body`,this.themeClass])},[(p(),x(w,{clsPrefix:t,style:m(e.default?``:this.cssVars),stroke:this.stroke,"stroke-width":this.mergedStrokeWidth,radius:this.radius,scale:this.scale,class:r(`${t}-spin`)},null,8,[`clsPrefix`,`style`,`stroke`,`stroke-width`,`radius`,`scale`,`class`])),c(()=>a)],2));return this.onRender?.(),e.default?(p(),y(`div`,{key:3,class:r([`${t}-spin-container`,this.themeClass]),style:m(this.cssVars)},[v(`div`,{class:r([`${t}-spin-content`,this.active&&`${t}-spin-content--spinning`,this.contentClass]),style:m(this.contentStyle)},[c(()=>e.default?.())],6),g(S,{name:`fade-in-transition`},{default:()=>this.active?o:null},1024)],6)):o}});export{k as t};