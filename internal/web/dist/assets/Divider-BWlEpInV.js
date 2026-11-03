import{at as e,c as t,ct as n,g as r,l as i,lt as a,st as o,w as s,y as c}from"./resolve-slot-D0ESJhJD.js";import{mt as l}from"./user-ZPntuAC8.js";import{S as u,V as d,Wt as f,d as p,f as m,h,i as g}from"./runtime-core.esm-bundler-B5qSvmKr.js";var _=e(`divider`,`
 position: relative;
 display: flex;
 width: 100%;
 box-sizing: border-box;
 font-size: 16px;
 color: var(--n-text-color);
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
`,[a(`vertical`,`
 margin-top: 24px;
 margin-bottom: 24px;
 `,[a(`no-title`,`
 display: flex;
 align-items: center;
 `)]),o(`title`,`
 display: flex;
 align-items: center;
 margin-left: 12px;
 margin-right: 12px;
 white-space: nowrap;
 font-weight: var(--n-font-weight);
 `),n(`title-position-left`,[o(`line`,[n(`left`,{width:`28px`})])]),n(`title-position-right`,[o(`line`,[n(`right`,{width:`28px`})])]),n(`dashed`,[o(`line`,`
 background-color: #0000;
 height: 0px;
 width: 100%;
 border-style: dashed;
 border-width: 1px 0 0;
 `)]),n(`vertical`,`
 display: inline-block;
 height: 1em;
 margin: 0 8px;
 vertical-align: middle;
 width: 1px;
 `),o(`line`,`
 border: none;
 transition: background-color .3s var(--n-bezier), border-color .3s var(--n-bezier);
 height: 1px;
 width: 100%;
 margin: 0;
 `),a(`dashed`,[o(`line`,{backgroundColor:`var(--n-color)`})]),n(`dashed`,[o(`line`,{borderColor:`var(--n-color)`})]),n(`vertical`,{backgroundColor:`var(--n-color)`})]),v={...t.props,titlePlacement:{type:String,default:`center`},dashed:Boolean,vertical:Boolean},y=u({name:`Divider`,props:v,setup(e){let{mergedClsPrefixRef:n,inlineThemeDisabled:r}=s(e),a=t(`Divider`,`-divider`,_,l,e,n),o=p(()=>{let{common:{cubicBezierEaseInOut:e},self:{color:t,textColor:n,fontWeight:r}}=a.value;return{"--n-bezier":e,"--n-color":t,"--n-text-color":n,"--n-font-weight":r}}),c=r?i(`divider`,void 0,o,e):void 0;return{mergedClsPrefix:n,cssVars:r?void 0:o,themeClass:c?.themeClass,onRender:c?.onRender}},render(){let{$slots:e,titlePlacement:t,vertical:n,dashed:i,cssVars:a,mergedClsPrefix:o}=this;return this.onRender?.(),d(),h(`div`,{role:`separator`,class:r([`${o}-divider`,this.themeClass,{[`${o}-divider--vertical`]:n,[`${o}-divider--no-title`]:!e.default,[`${o}-divider--dashed`]:i,[`${o}-divider--title-position-${t}`]:e.default&&t}]),style:f(a)},[n?c(()=>null):(d(),h(`div`,{key:0,class:r(`${o}-divider__line ${o}-divider__line--left`)},null,2)),!n&&e.default?(d(),h(g,{key:2},[m(`div`,{class:r(`${o}-divider__title`)},[c(()=>this.$slots.default?.())],2),m(`div`,{class:r(`${o}-divider__line ${o}-divider__line--right`)},null,2)],64)):c(()=>null)],6)}});export{y as t};