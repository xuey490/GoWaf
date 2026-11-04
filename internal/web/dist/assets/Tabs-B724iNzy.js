import{D as e,Z as t,a as n,at as r,b as i,c as a,ct as o,g as s,it as c,k as l,l as u,lt as d,nt as f,st as p,ut as m,v as h,w as g,y as _}from"./resolve-slot-D0ESJhJD.js";import{n as v,t as y}from"./use-rtl-B41EZXdW.js";import{p as b}from"./light-D4diJaDS.js";import{lt as x,wt as S}from"./user-eoa71Een.js";import{a as C,l as w,n as T}from"./replaceable-CG1Qs4lR.js";import{r as E}from"./Scrollbar-D9NqNphl.js";import{E as D,H as O,M as k,O as A,R as j,S as M,St as N,V as P,Wt as F,_t as I,d as L,f as R,h as z,i as B,it as ee,j as V,ot as te,p as H,rt as U,u as ne}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{a as W,n as G}from"./runtime-dom.esm-bundler-B3Pl3ttw.js";import{t as re}from"./on-fonts-ready-Clf4bkGj.js";import{t as ie}from"./use-merged-state-KQOIh2IK.js";import{t as ae}from"./use-compitable-CgN8vuBj.js";import{t as oe}from"./flatten-q_1Pdy7y.js";import{n as K,t as q}from"./cssr-dSjIL54B.js";import{t as se}from"./Close-swctlKIK.js";import{t as ce}from"./Button-DUNxlS0h.js";import{t as le}from"./ChevronLeft-Bb8WFTYP.js";import{t as ue}from"./ChevronRight-CSGCWNSb.js";import{t as de}from"./Add-DQqGLM7W.js";var fe=/\s/;function pe(e){for(var t=e.length;t--&&fe.test(e.charAt(t)););return t}var me=/^\s+/;function he(e){return e&&e.slice(0,pe(e)+1).replace(me,``)}var J=NaN,ge=/^[-+]0x[0-9a-f]+$/i,Y=/^0b[01]+$/i,X=/^0o[0-7]+$/i,_e=parseInt;function ve(e){if(typeof e==`number`)return e;if(w(e))return J;if(t(e)){var n=typeof e.valueOf==`function`?e.valueOf():e;e=t(n)?n+``:n}if(typeof e!=`string`)return e===0?e:+e;e=he(e);var r=Y.test(e);return r||X.test(e)?_e(e.slice(2),r?2:8):ge.test(e)?J:+e}var ye=function(){return f.Date.now()},be=`Expected a function`,Z=Math.max,xe=Math.min;function Se(e,n,r){var i,a,o,s,c,l,u=0,d=!1,f=!1,p=!0;if(typeof e!=`function`)throw TypeError(be);n=ve(n)||0,t(r)&&(d=!!r.leading,f=`maxWait`in r,o=f?Z(ve(r.maxWait)||0,n):o,p=`trailing`in r?!!r.trailing:p);function m(t){var n=i,r=a;return i=a=void 0,u=t,s=e.apply(r,n),s}function h(e){return u=e,c=setTimeout(v,n),d?m(e):s}function g(e){var t=e-l,r=e-u,i=n-t;return f?xe(i,o-r):i}function _(e){var t=e-l,r=e-u;return l===void 0||t>=n||t<0||f&&r>=o}function v(){var e=ye();if(_(e))return y(e);c=setTimeout(v,g(e))}function y(e){return c=void 0,p&&i?m(e):(i=a=void 0,s)}function b(){c!==void 0&&clearTimeout(c),u=0,i=l=a=c=void 0}function x(){return c===void 0?s:y(ye())}function S(){var e=ye(),t=_(e);if(i=arguments,a=this,l=e,t){if(c===void 0)return h(l);if(f)return clearTimeout(c),c=setTimeout(v,n),m(l)}return c===void 0&&(c=setTimeout(v,n)),s}return S.cancel=b,S.flush=x,S}var Ce=`Expected a function`;function we(e,n,r){var i=!0,a=!0;if(typeof e!=`function`)throw TypeError(Ce);return t(r)&&(i=`leading`in r?!!r.leading:i,a=`trailing`in r?!!r.trailing:a),Se(e,n,{leading:i,maxWait:n,trailing:a})}var Te=q(`.v-x-scroll`,{overflow:`auto`,scrollbarWidth:`none`},[q(`&::-webkit-scrollbar`,{width:0,height:0})]),Q=M({name:`XScroll`,props:{disabled:Boolean,onScroll:Function},setup(){let e=I(null);function t(e){e.currentTarget.offsetWidth<e.currentTarget.scrollWidth&&e.deltaY!==0&&(e.currentTarget.scrollLeft+=e.deltaY+e.deltaX,e.preventDefault())}let n=i();return Te.mount({id:`vueuc/x-scroll`,head:!0,anchorMetaName:K,ssr:n}),Object.assign({selfRef:e,handleWheel:t},{scrollTo(...t){var n;(n=e.value)==null||n.scrollTo(...t)}})},render(){return D(`div`,{ref:`selfRef`,onScroll:this.onScroll,onWheel:this.disabled?void 0:this.handleWheel,class:`v-x-scroll`},this.$slots)}}),Ee=e(`n-tabs`),De={tab:[String,Number,Object,Function],name:{type:[String,Number],required:!0},disabled:Boolean,displayDirective:{type:String,default:`if`},closable:{type:Boolean,default:void 0},tabProps:Object,label:[String,Number,Object,Function]},Oe=M({__TAB_PANE__:!0,name:`TabPane`,alias:[`TabPanel`],props:De,slots:Object,setup(e){let t=A(Ee,null);return t||l(`tab-pane`,"`n-tab-pane` must be placed inside `n-tabs`."),{style:t.paneStyleRef,class:t.paneClassRef,mergedClsPrefix:t.mergedClsPrefixRef}},render(){return P(),z(`div`,{class:s([`${this.mergedClsPrefix}-tab-pane`,this.class]),style:F(this.style)},[_(()=>this.$slots.default?.())],6)}}),ke=[`data-name`,`data-disabled`],Ae={internalLeftPadded:Boolean,internalAddable:Boolean,internalCreatedByPane:Boolean,...S(De,[`displayDirective`])},$=M({__TAB__:!0,inheritAttrs:!1,name:`Tab`,props:Ae,setup(e){let{mergedClsPrefixRef:t,valueRef:n,typeRef:r,closableRef:i,tabStyleRef:a,addTabStyleRef:o,tabClassRef:s,addTabClassRef:c,tabChangeIdRef:l,onBeforeLeaveRef:u,triggerRef:d,handleAdd:f,activateTab:p,handleClose:m}=A(Ee);return{trigger:d,mergedClosable:L(()=>{if(e.internalAddable)return!1;let{closable:t}=e;return t===void 0?i.value:t}),style:a,addStyle:o,tabClass:s,addTabClass:c,clsPrefix:t,value:n,type:r,handleClose(t){t.stopPropagation(),!e.disabled&&m(e.name)},activateTab(){if(e.disabled)return;if(e.internalAddable){f();return}let{name:t}=e,r=++l.id;if(t!==n.value){let{value:i}=u;i?Promise.resolve(i(e.name,n.value)).then(e=>{e&&l.id===r&&p(t)}):p(t)}}}},render(){let{internalAddable:e,clsPrefix:t,name:n,disabled:r,label:i,tab:a,value:o,mergedClosable:c,trigger:l,$slots:{default:u}}=this,d=i??a;return P(),z(`div`,{class:s(`${t}-tabs-tab-wrapper`)},[this.internalLeftPadded?(P(),z(`div`,{key:0,class:s(`${t}-tabs-tab-pad`)},null,2)):_(()=>null),(P(),z(`div`,V({key:n,"data-name":n,"data-disabled":r?!0:void 0},V({class:[`${t}-tabs-tab`,o===n&&`${t}-tabs-tab--active`,r&&`${t}-tabs-tab--disabled`,c&&`${t}-tabs-tab--closable`,e&&`${t}-tabs-tab--addable`,e?this.addTabClass:this.tabClass],onClick:l===`click`?this.activateTab:void 0,onMouseenter:l===`hover`?this.activateTab:void 0,style:e?this.addStyle:this.style},this.internalCreatedByPane?this.tabProps||{}:this.$attrs)),[R(`span`,{class:s(`${t}-tabs-tab__label`)},[e?(P(),z(B,{key:0},[R(`div`,{class:s(`${t}-tabs-tab__height-placeholder`)},`\xA0`,2),(P(),H(T,{clsPrefix:t},{default:()=>(P(),H(de))},1032,[`clsPrefix`]))],64)):(P(),z(B,{key:1},[u?(P(),z(B,{key:0},[_(()=>u())],64)):(P(),z(B,{key:1},[typeof d==`object`?(P(),z(B,{key:0},[_(()=>d)],64)):(P(),z(B,{key:1},[_(()=>b(d??n))],64))],64))],64))],2),c&&this.type===`card`?(P(),H(se,{key:0,clsPrefix:t,class:s(`${t}-tabs-tab__close`),onClick:this.handleClose,disabled:r},null,8,[`clsPrefix`,`class`,`onClick`,`disabled`])):_(()=>null)],16,ke))],2)}}),je=r(`tabs`,`
 box-sizing: border-box;
 width: 100%;
 display: flex;
 flex-direction: column;
 transition:
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
`,[c(`&.transition-disabled`,[r(`tabs-tab`,`
 transition: none !important;
 `),r(`tabs-nav-scroll-content`,`
 transition: none !important;
 `),r(`tabs-tab-pad`,`
 transition: none !important;
 `)]),o(`segment-type`,[r(`tabs-rail`,[c(`&.transition-disabled`,[r(`tabs-capsule`,`
 transition: none;
 `)])])]),o(`top`,[r(`tab-pane`,`
 padding: var(--n-pane-padding-top) var(--n-pane-padding-right) var(--n-pane-padding-bottom) var(--n-pane-padding-left);
 `)]),o(`left`,[r(`tab-pane`,`
 padding: var(--n-pane-padding-right) var(--n-pane-padding-bottom) var(--n-pane-padding-left) var(--n-pane-padding-top);
 `)]),o(`left, right`,`
 flex-direction: row;
 `,[r(`tabs-bar`,`
 width: 2px;
 right: 0;
 transition:
 top .2s var(--n-bezier),
 max-height .2s var(--n-bezier),
 background-color .3s var(--n-bezier);
 `),r(`tabs-tab`,`
 padding: var(--n-tab-padding-vertical); 
 `)]),o(`right`,`
 flex-direction: row-reverse;
 `,[r(`tab-pane`,`
 padding: var(--n-pane-padding-left) var(--n-pane-padding-top) var(--n-pane-padding-right) var(--n-pane-padding-bottom);
 `),r(`tabs-bar`,`
 left: 0;
 `)]),o(`bottom`,`
 flex-direction: column-reverse;
 justify-content: flex-end;
 `,[r(`tab-pane`,`
 padding: var(--n-pane-padding-bottom) var(--n-pane-padding-right) var(--n-pane-padding-top) var(--n-pane-padding-left);
 `),r(`tabs-bar`,`
 top: 0;
 `)]),r(`tabs-rail`,`
 position: relative;
 padding: 3px;
 border-radius: var(--n-tab-border-radius);
 width: 100%;
 background-color: var(--n-color-segment);
 transition: background-color .3s var(--n-bezier);
 display: flex;
 align-items: center;
 `,[r(`tabs-capsule`,`
 border-radius: var(--n-tab-border-radius);
 position: absolute;
 left: 0;
 top: 0;
 pointer-events: none;
 background-color: var(--n-tab-color-segment);
 box-shadow: 0 1px 3px 0 rgba(0, 0, 0, .08);
 transition: transform 0.3s var(--n-bezier);
 `),r(`tabs-tab-wrapper`,`
 flex-basis: 0;
 flex-grow: 1;
 display: flex;
 align-items: center;
 justify-content: center;
 `,[r(`tabs-tab`,`
 overflow: hidden;
 border-radius: var(--n-tab-border-radius);
 width: 100%;
 display: flex;
 align-items: center;
 justify-content: center;
 `,[o(`active`,`
 font-weight: var(--n-font-weight-strong);
 color: var(--n-tab-text-color-active);
 `),c(`&:hover`,`
 color: var(--n-tab-text-color-hover);
 `)])])]),o(`flex`,[r(`tabs-nav`,`
 width: 100%;
 position: relative;
 `,[r(`tabs-wrapper`,`
 width: 100%;
 `,[r(`tabs-tab`,`
 margin-right: 0;
 `)])])]),r(`tabs-nav`,`
 box-sizing: border-box;
 line-height: 1.5;
 display: flex;
 transition: border-color .3s var(--n-bezier);
 `,[p(`prefix, suffix`,`
 display: flex;
 align-items: center;
 `),p(`prefix`,`padding-right: 16px;`),p(`suffix`,`padding-left: 16px;`)]),o(`top, bottom`,[c(`>`,[r(`tabs-nav`,[r(`tabs-nav-scroll-wrapper`,[c(`&::before`,`
 top: 0;
 bottom: 0;
 left: 0;
 width: 20px;
 `),c(`&::after`,`
 top: 0;
 bottom: 0;
 right: 0;
 width: 20px;
 `),o(`shadow-start`,[c(`&::before`,`
 box-shadow: inset 10px 0 8px -8px rgba(0, 0, 0, .12);
 `)]),o(`shadow-end`,[c(`&::after`,`
 box-shadow: inset -10px 0 8px -8px rgba(0, 0, 0, .12);
 `)])])])])]),o(`left, right`,[r(`tabs-nav-scroll-content`,`
 flex-direction: column;
 `),c(`>`,[r(`tabs-nav`,[r(`tabs-nav-scroll-wrapper`,[c(`&::before`,`
 top: 0;
 left: 0;
 right: 0;
 height: 20px;
 `),c(`&::after`,`
 bottom: 0;
 left: 0;
 right: 0;
 height: 20px;
 `),o(`shadow-start`,[c(`&::before`,`
 box-shadow: inset 0 10px 8px -8px rgba(0, 0, 0, .12);
 `)]),o(`shadow-end`,[c(`&::after`,`
 box-shadow: inset 0 -10px 8px -8px rgba(0, 0, 0, .12);
 `)])])])])]),r(`tabs-nav-scroll-wrapper`,`
 flex: 1;
 position: relative;
 overflow: hidden;
 `,[r(`tabs-nav-y-scroll`,`
 height: 100%;
 width: 100%;
 overflow-y: auto; 
 scrollbar-width: none;
 `,[c(`&::-webkit-scrollbar, &::-webkit-scrollbar-track-piece, &::-webkit-scrollbar-thumb`,`
 width: 0;
 height: 0;
 display: none;
 `)]),c(`&::before, &::after`,`
 transition: box-shadow .3s var(--n-bezier);
 pointer-events: none;
 content: "";
 position: absolute;
 z-index: 1;
 `),c(`&.transition-disabled`,[c(`&::before, &::after`,`
 transition: none;
 `)])]),r(`tabs-nav-scroll-content`,`
 display: flex;
 position: relative;
 min-width: 100%;
 min-height: 100%;
 width: fit-content;
 box-sizing: border-box;
 `),r(`tabs-wrapper`,`
 display: inline-flex;
 flex-wrap: nowrap;
 position: relative;
 `),r(`tabs-tab-wrapper`,`
 display: flex;
 flex-wrap: nowrap;
 flex-shrink: 0;
 flex-grow: 0;
 `),r(`tabs-tab`,`
 cursor: pointer;
 white-space: nowrap;
 flex-wrap: nowrap;
 display: inline-flex;
 align-items: center;
 color: var(--n-tab-text-color);
 font-size: var(--n-tab-font-size);
 background-clip: padding-box;
 padding: var(--n-tab-padding);
 transition:
 box-shadow .3s var(--n-bezier),
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `,[o(`disabled`,{cursor:`not-allowed`}),p(`close`,`
 margin-inline-start: 6px;
 transition:
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
 `),p(`label`,`
 display: flex;
 align-items: center;
 z-index: 1;
 `)]),r(`tabs-bar`,`
 position: absolute;
 bottom: 0;
 height: 2px;
 border-radius: 1px;
 background-color: var(--n-bar-color);
 transition:
 left .2s var(--n-bezier),
 max-width .2s var(--n-bezier),
 opacity .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 `,[c(`&.transition-disabled`,`
 transition: none;
 `),o(`disabled`,`
 background-color: var(--n-tab-text-color-disabled)
 `)]),r(`tabs-pane-wrapper`,`
 position: relative;
 overflow: hidden;
 transition: max-height .2s var(--n-bezier);
 `),r(`tab-pane`,`
 color: var(--n-pane-text-color);
 width: 100%;
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 opacity .2s var(--n-bezier);
 left: 0;
 right: 0;
 top: 0;
 `,[c(`&.next-transition-leave-active, &.prev-transition-leave-active, &.next-transition-enter-active, &.prev-transition-enter-active`,`
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 transform .2s var(--n-bezier),
 opacity .2s var(--n-bezier);
 `),c(`&.next-transition-leave-active, &.prev-transition-leave-active`,`
 position: absolute;
 `),c(`&.next-transition-enter-from, &.prev-transition-leave-to`,`
 transform: translateX(32px);
 opacity: 0;
 `),c(`&.next-transition-leave-to, &.prev-transition-enter-from`,`
 transform: translateX(-32px);
 opacity: 0;
 `),c(`&.next-transition-leave-from, &.next-transition-enter-to, &.prev-transition-leave-from, &.prev-transition-enter-to`,`
 transform: translateX(0);
 opacity: 1;
 `)]),r(`tabs-tab-pad`,`
 box-sizing: border-box;
 width: var(--n-tab-gap);
 flex-grow: 0;
 flex-shrink: 0;
 `),o(`line-type, bar-type`,[r(`tabs-tab`,`
 font-weight: var(--n-tab-font-weight);
 box-sizing: border-box;
 vertical-align: bottom;
 `,[c(`&:hover`,{color:`var(--n-tab-text-color-hover)`}),o(`active`,`
 color: var(--n-tab-text-color-active);
 font-weight: var(--n-tab-font-weight-active);
 `),o(`disabled`,{color:`var(--n-tab-text-color-disabled)`})])]),r(`tabs-nav`,[p(`prefix, suffix`,`
 border-color: var(--n-tab-border-color);
 `),r(`tabs-nav-scroll-content`,`
 border-color: var(--n-tab-border-color);
 `),o(`line-type`,[o(`top`,[p(`prefix, suffix`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `),r(`tabs-nav-scroll-content`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `),r(`tabs-bar`,`
 bottom: -1px;
 `)]),o(`left`,[p(`prefix, suffix`,`
 border-right: 1px solid var(--n-tab-border-color);
 `),r(`tabs-nav-scroll-content`,`
 border-right: 1px solid var(--n-tab-border-color);
 `),r(`tabs-bar`,`
 right: -1px;
 `)]),o(`right`,[p(`prefix, suffix`,`
 border-left: 1px solid var(--n-tab-border-color);
 `),r(`tabs-nav-scroll-content`,`
 border-left: 1px solid var(--n-tab-border-color);
 `),r(`tabs-bar`,`
 left: -1px;
 `)]),o(`bottom`,[p(`prefix, suffix`,`
 border-top: 1px solid var(--n-tab-border-color);
 `),r(`tabs-nav-scroll-content`,`
 border-top: 1px solid var(--n-tab-border-color);
 `),r(`tabs-bar`,`
 top: -1px;
 `)]),p(`prefix, suffix`,`
 transition: border-color .3s var(--n-bezier);
 `),r(`tabs-nav-scroll-content`,`
 transition: border-color .3s var(--n-bezier);
 `),r(`tabs-bar`,`
 border-radius: 0;
 `)]),o(`card-type`,[p(`prefix, suffix`,`
 transition: border-color .3s var(--n-bezier);
 `),r(`tabs-pad`,`
 flex-grow: 1;
 transition: border-color .3s var(--n-bezier);
 `),r(`tabs-tab-pad`,`
 transition: border-color .3s var(--n-bezier);
 `),r(`tabs-tab`,`
 font-weight: var(--n-tab-font-weight);
 border: 1px solid var(--n-tab-border-color);
 background-color: var(--n-tab-color);
 box-sizing: border-box;
 position: relative;
 vertical-align: bottom;
 display: flex;
 justify-content: space-between;
 font-size: var(--n-tab-font-size);
 color: var(--n-tab-text-color);
 `,[o(`addable`,`
 padding-left: 8px;
 padding-right: 8px;
 font-size: 16px;
 justify-content: center;
 `,[p(`height-placeholder`,`
 width: 0;
 font-size: var(--n-tab-font-size);
 `),d(`disabled`,[c(`&:hover`,`
 color: var(--n-tab-text-color-hover);
 `)])]),o(`closable`,`padding-inline-end: 8px;`),o(`active`,`
 background-color: #0000;
 font-weight: var(--n-tab-font-weight-active);
 color: var(--n-tab-text-color-active);
 `),o(`disabled`,`color: var(--n-tab-text-color-disabled);`)])]),o(`left, right`,`
 flex-direction: column; 
 `,[p(`prefix, suffix`,`
 padding: var(--n-tab-padding-vertical);
 `),r(`tabs-wrapper`,`
 flex-direction: column;
 `),r(`tabs-tab-wrapper`,`
 flex-direction: column;
 `,[r(`tabs-tab-pad`,`
 height: var(--n-tab-gap-vertical);
 width: 100%;
 `)])]),o(`top`,[o(`card-type`,[r(`tabs-scroll-padding`,`border-bottom: 1px solid var(--n-tab-border-color);`),p(`prefix, suffix`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `),r(`tabs-tab`,`
 border-top-left-radius: var(--n-tab-border-radius);
 border-top-right-radius: var(--n-tab-border-radius);
 `,[o(`active`,`
 border-bottom: 1px solid #0000;
 `)]),r(`tabs-tab-pad`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `),r(`tabs-pad`,`
 border-bottom: 1px solid var(--n-tab-border-color);
 `)])]),o(`left`,[o(`card-type`,[r(`tabs-scroll-padding`,`border-right: 1px solid var(--n-tab-border-color);`),p(`prefix, suffix`,`
 border-right: 1px solid var(--n-tab-border-color);
 `),r(`tabs-tab`,`
 border-top-left-radius: var(--n-tab-border-radius);
 border-bottom-left-radius: var(--n-tab-border-radius);
 `,[o(`active`,`
 border-right: 1px solid #0000;
 `)]),r(`tabs-tab-pad`,`
 border-right: 1px solid var(--n-tab-border-color);
 `),r(`tabs-pad`,`
 border-right: 1px solid var(--n-tab-border-color);
 `)])]),o(`right`,[o(`card-type`,[r(`tabs-scroll-padding`,`border-left: 1px solid var(--n-tab-border-color);`),p(`prefix, suffix`,`
 border-left: 1px solid var(--n-tab-border-color);
 `),r(`tabs-tab`,`
 border-top-right-radius: var(--n-tab-border-radius);
 border-bottom-right-radius: var(--n-tab-border-radius);
 `,[o(`active`,`
 border-left: 1px solid #0000;
 `)]),r(`tabs-tab-pad`,`
 border-left: 1px solid var(--n-tab-border-color);
 `),r(`tabs-pad`,`
 border-left: 1px solid var(--n-tab-border-color);
 `)])]),o(`bottom`,[o(`card-type`,[r(`tabs-scroll-padding`,`border-top: 1px solid var(--n-tab-border-color);`),p(`prefix, suffix`,`
 border-top: 1px solid var(--n-tab-border-color);
 `),r(`tabs-tab`,`
 border-bottom-left-radius: var(--n-tab-border-radius);
 border-bottom-right-radius: var(--n-tab-border-radius);
 `,[o(`active`,`
 border-top: 1px solid #0000;
 `)]),r(`tabs-tab-pad`,`
 border-top: 1px solid var(--n-tab-border-color);
 `),r(`tabs-pad`,`
 border-top: 1px solid var(--n-tab-border-color);
 `)])])]),r(`tabs-scroll-button`,[o(`start`,`
 padding-left: 10px;
 padding-right: 6px;
 `),o(`end`,`
 padding-right: 10px;
 padding-left: 6px;
 `),o(`up`,`
 padding-bottom: 10px;
 `),o(`down`,`
 padding-top: 10px;
 `)])]),Me=M({name:`TabsButton`,props:{type:{type:String,default:`next`},mergedClsPrefix:{type:String,required:!0},vertical:Boolean,disabled:Boolean,rtl:Boolean,theme:Object,themeOverrides:Object,onClick:Function},setup(e){return{handleClick:()=>{e.disabled||e.onClick?.(e.type)}}},render(){let{mergedClsPrefix:e,disabled:t,type:n,vertical:r,rtl:i,theme:a,themeOverrides:o,handleClick:c}=this,l=n===`next`,u=r?l:i?!l:l;return P(),H(ce,{text:!0,disabled:t,size:`small`,theme:a,themeOverrides:o,onClick:c,class:s([`${e}-tabs-scroll-button`,!r&&n===`prev`&&`${e}-tabs-scroll-button--start`,!r&&n===`next`&&`${e}-tabs-scroll-button--end`,r&&n===`prev`&&`${e}-tabs-scroll-button--up`,r&&n===`next`&&`${e}-tabs-scroll-button--down`])},{icon:()=>(P(),H(T,{clsPrefix:e,style:F(r?{transform:`rotate(90deg)`}:void 0)},{default:()=>u?(P(),H(ue,{key:1})):(P(),H(le,{key:2}))},1032,[`clsPrefix`,`style`]))},1032,[`disabled`,`theme`,`themeOverrides`,`onClick`,`class`])}}),Ne=we,Pe={...a.props,value:[String,Number],defaultValue:[String,Number],trigger:{type:String,default:`click`},type:{type:String,default:`bar`},closable:Boolean,justifyContent:String,size:String,placement:{type:String,default:`top`},tabStyle:[String,Object],tabClass:String,addTabStyle:[String,Object],addTabClass:String,barWidth:Number,paneClass:String,paneStyle:[String,Object],paneWrapperClass:String,paneWrapperStyle:[String,Object],addable:[Boolean,Object],tabsPadding:{type:Number,default:0},animated:Boolean,onBeforeLeave:Function,onAdd:Function,"onUpdate:value":[Function,Array],onUpdateValue:[Function,Array],onClose:[Function,Array],labelSize:String,activeName:[String,Number],onActiveNameChange:[Function,Array],showScrollButton:Boolean,centerActiveTab:Boolean},Fe=M({name:`Tabs`,props:Pe,slots:Object,setup(e,{slots:t}){let{mergedClsPrefixRef:n,inlineThemeDisabled:r,mergedComponentPropsRef:i,mergedRtlRef:o}=g(e),s=y(`Tabs`,o,n),c=L(()=>{let{placement:t}=e;return t===`start`?s?.value?`right`:`left`:t===`end`?s?.value?`left`:`right`:t}),l=a(`Tabs`,`-tabs`,je,x,e,n),d=I(null),f=I(null),p=I(null),h=I(null),_=I(null),b=I(null),S=I(null),w=I(!0),T=I(!0),E=ae(e,[`labelSize`,`size`]),D=L(()=>E.value?E.value:i?.value?.Tabs?.size||`medium`),A=ae(e,[`activeName`,`value`]),M=I(A.value??e.defaultValue??(t.default?oe(t.default())[0]?.props?.name:null)),P=ie(A,M),F={id:0},R=L(()=>{if(e.justifyContent&&e.type!==`card`)return{display:`flex`,justifyContent:e.justifyContent}});U(P,()=>{F.id=0,H(),k(()=>{W()})});function z(){let{value:e}=P;return e===null?null:d.value?.querySelector(`[data-name="${e}"]`)}function B(t){if(e.type===`card`)return;let{value:r}=p;if(!r)return;let i=r.style.opacity===`0`;if(t){let a=`${n.value}-tabs-bar--disabled`,{barWidth:o}=e,s=c.value;if(t.dataset.disabled===`true`?r.classList.add(a):r.classList.remove(a),[`top`,`bottom`].includes(s)){if(te([`top`,`maxHeight`,`height`]),typeof o==`number`&&t.offsetWidth>=o){let e=Math.floor((t.offsetWidth-o)/2)+t.offsetLeft;r.style.left=`${e}px`,r.style.maxWidth=`${o}px`}else r.style.left=`${t.offsetLeft}px`,r.style.maxWidth=`${t.offsetWidth}px`;r.style.width=`8192px`,i&&(r.style.transition=`none`),r.offsetWidth,i&&(r.style.transition=``,r.style.opacity=`1`)}else{if(te([`left`,`maxWidth`,`width`]),typeof o==`number`&&t.offsetHeight>=o){let e=Math.floor((t.offsetHeight-o)/2)+t.offsetTop;r.style.top=`${e}px`,r.style.maxHeight=`${o}px`}else r.style.top=`${t.offsetTop}px`,r.style.maxHeight=`${t.offsetHeight}px`;r.style.height=`8192px`,i&&(r.style.transition=`none`),r.offsetHeight,i&&(r.style.transition=``,r.style.opacity=`1`)}}}function V(){if(e.type===`card`)return;let{value:t}=p;t&&(t.style.opacity=`0`)}function te(e){let{value:t}=p;if(t)for(let n of e)t.style[n]=``}function H(){if(e.type===`card`)return;let t=z();t?B(t):V()}function ne(e,t,n,r){let i=e.getBoundingClientRect(),a=t.getBoundingClientRect(),o=n?`left`:`top`,s=n?`right`:`bottom`,c=0;r?c=(a[o]+a[s])/2-(i[o]+i[s])/2:a[o]<i[o]?c=a[o]-i[o]:a[s]>i[s]&&(c=a[s]-i[s]),c!==0&&e.scrollBy({[o]:c,behavior:`smooth`})}function W(){let t=[`top`,`bottom`].includes(c.value),n=z();if(n){if(t){let r=b.value?.$el;if(!r)return;ne(r,n,t,e.centerActiveTab)}else{let{value:r}=S;if(!r)return;ne(r,n,t,e.centerActiveTab)}}}let G=I(null),K=0,q=null;function se(e){let t=G.value;if(t){K=e.getBoundingClientRect().height;let n=`${K}px`,r=()=>{t.style.height=n,t.style.maxHeight=n};q?(r(),q(),q=null):q=r}}function ce(e){let t=G.value;if(t){let n=e.getBoundingClientRect().height,r=()=>{document.body.offsetHeight,t.style.maxHeight=`${n}px`,t.style.height=`${Math.max(K,n)}px`};q?(q(),q=null,r()):q=r}}function le(){let t=G.value;if(t){t.style.maxHeight=``,t.style.height=``;let{paneWrapperStyle:n}=e;if(typeof n==`string`)t.style.cssText=n;else if(n){let{maxHeight:e,height:r}=n;e!==void 0&&(t.style.maxHeight=e),r!==void 0&&(t.style.height=r)}}}let ue={value:[]},de=I(`next`);function fe(e){let t=P.value,n=`next`;for(let r of ue.value){if(r===t)break;if(r===e){n=`prev`;break}}de.value=n,pe(e)}function pe(t){let{onActiveNameChange:n,onUpdateValue:r,"onUpdate:value":i}=e;n&&v(n,t),r&&v(r,t),i&&v(i,t),M.value=t}function me(t){let{onClose:n}=e;n&&v(n,t)}function he(e){if([`top`,`bottom`].includes(c.value)){let{value:t}=b;if(!t)return;let n=t.$el;if(!n)return;let r=n.offsetWidth,i=!!s?.value,a=e===`next`?r:-r;n.scrollBy({left:i?-a:a,behavior:`smooth`})}else{let{value:t}=S;if(!t)return;let n=t.offsetHeight,r=e===`next`?t.scrollTop+n:t.scrollTop-n;t.scrollTo({top:r,left:0,behavior:`smooth`})}}let J=!0;function ge(){let{value:e}=p;if(!e)return;J&&(J=!1);let t=`transition-disabled`;e.classList.add(t),H(),e.classList.remove(t)}let Y=I(null);function X({transitionDisabled:e}){let t=d.value;if(!t)return;e&&t.classList.add(`transition-disabled`);let n=z();n&&Y.value&&(Y.value.style.width=`${n.offsetWidth}px`,Y.value.style.height=`${n.offsetHeight}px`,Y.value.style.transform=`translate(${n.offsetLeft}px, ${n.offsetTop}px)`,e&&Y.value.offsetWidth),e&&t.classList.remove(`transition-disabled`)}U([P],()=>{e.type===`segment`&&k(()=>{X({transitionDisabled:!1})})}),j(()=>{e.type===`segment`&&X({transitionDisabled:!0})});let _e=0;function ve(t){if(t.contentRect.width===0&&t.contentRect.height===0||_e===t.contentRect.width)return;_e=t.contentRect.width;let{type:n}=e;(n===`line`||n===`bar`)&&(J||e.justifyContent?.startsWith(`space`))&&ge(),n!==`segment`&&Q(Te())}let ye=Ne(ve,64);function be(){let{type:t}=e;t===`line`||t===`bar`?ge():t===`segment`&&X({transitionDisabled:!0})}U([()=>e.justifyContent,()=>e.size],()=>{k(()=>{(e.type===`line`||e.type===`bar`)&&ge()})}),U([c,()=>s?.value],()=>{k(()=>{be(),Q(Te(),{instantly:!0})})}),U(()=>e.type,()=>{k(()=>{let e=f.value;e&&(e.classList.add(`transition-disabled`),be(),e.offsetWidth,e.classList.remove(`transition-disabled`))})});let Z=I(!1);function xe(e){let{target:t,contentRect:{width:n,height:r}}=e,i=t.parentElement.parentElement.offsetWidth,a=t.parentElement.parentElement.offsetHeight,o=c.value;if(!Z.value)o===`top`||o===`bottom`?i<n&&(Z.value=!0):a<r&&(Z.value=!0);else{let{value:e}=_;if(!e)return;o===`top`||o===`bottom`?i-n>e.$el.offsetWidth&&(Z.value=!1):a-r>e.$el.offsetHeight&&(Z.value=!1)}Q(b.value?.$el||null)}let Se=Ne(xe,64);function Ce(){let{onAdd:t}=e;t&&t()}let we=I(!1);function Te(){let e=c.value;return(e===`top`||e===`bottom`?b.value?.$el:S.value)||null}function Q(e,t={instantly:!1}){if(!e)return;let n=t.instantly?h.value:null;n&&n.classList.add(`transition-disabled`);let r=c.value;if(r===`top`||r===`bottom`){let{scrollLeft:t,scrollWidth:n,offsetWidth:r}=e,i=Math.abs(t);w.value=i<=1,T.value=i+r>=n-1,we.value=r<n-1}else{let{scrollTop:t,scrollHeight:n,offsetHeight:r}=e;w.value=t<=1,T.value=t+r>=n-1,we.value=r<n-1}n&&(n.offsetWidth,n.classList.remove(`transition-disabled`))}let De=Ne(e=>{Q(e.target)},64);O(Ee,{triggerRef:N(e,`trigger`),tabStyleRef:N(e,`tabStyle`),tabClassRef:N(e,`tabClass`),addTabStyleRef:N(e,`addTabStyle`),addTabClassRef:N(e,`addTabClass`),paneClassRef:N(e,`paneClass`),paneStyleRef:N(e,`paneStyle`),mergedClsPrefixRef:n,typeRef:N(e,`type`),closableRef:N(e,`closable`),valueRef:P,tabChangeIdRef:F,onBeforeLeaveRef:N(e,`onBeforeLeave`),activateTab:fe,handleClose:me,handleAdd:Ce}),re(()=>{H(),W()}),ee(()=>{let{value:e}=h;if(!e)return;let{value:t}=n,r=`${t}-tabs-nav-scroll-wrapper--shadow-start`,i=`${t}-tabs-nav-scroll-wrapper--shadow-end`;w.value?e.classList.remove(r):e.classList.add(r),T.value?e.classList.remove(i):e.classList.add(i)});let Oe={syncBarPosition:()=>{H()},scrollToCurrentTab:()=>{W()}},ke=()=>{X({transitionDisabled:!0})},Ae=L(()=>{let{value:t}=D,{type:n}=e,r=`${t}${{card:`Card`,bar:`Bar`,line:`Line`,segment:`Segment`}[n]}`,{self:{barColor:i,closeIconColor:a,closeIconColorHover:o,closeIconColorPressed:s,tabColor:c,tabBorderColor:u,paneTextColor:d,tabFontWeight:f,tabBorderRadius:p,tabFontWeightActive:h,colorSegment:g,fontWeightStrong:_,tabColorSegment:v,closeSize:y,closeIconSize:b,closeColorHover:x,closeColorPressed:S,closeBorderRadius:w,[m(`panePadding`,t)]:T,[m(`tabPadding`,r)]:E,[m(`tabPaddingVertical`,r)]:O,[m(`tabGap`,r)]:k,[m(`tabGap`,`${r}Vertical`)]:A,[m(`tabTextColor`,n)]:j,[m(`tabTextColorActive`,n)]:M,[m(`tabTextColorHover`,n)]:N,[m(`tabTextColorDisabled`,n)]:P,[m(`tabFontSize`,t)]:F},common:{cubicBezierEaseInOut:I}}=l.value;return{"--n-bezier":I,"--n-color-segment":g,"--n-bar-color":i,"--n-tab-font-size":F,"--n-tab-text-color":j,"--n-tab-text-color-active":M,"--n-tab-text-color-disabled":P,"--n-tab-text-color-hover":N,"--n-pane-text-color":d,"--n-tab-border-color":u,"--n-tab-border-radius":p,"--n-close-size":y,"--n-close-icon-size":b,"--n-close-color-hover":x,"--n-close-color-pressed":S,"--n-close-border-radius":w,"--n-close-icon-color":a,"--n-close-icon-color-hover":o,"--n-close-icon-color-pressed":s,"--n-tab-color":c,"--n-tab-font-weight":f,"--n-tab-font-weight-active":h,"--n-tab-padding":E,"--n-tab-padding-vertical":O,"--n-tab-gap":k,"--n-tab-gap-vertical":A,"--n-pane-padding-left":C(T,`left`),"--n-pane-padding-right":C(T,`right`),"--n-pane-padding-top":C(T,`top`),"--n-pane-padding-bottom":C(T,`bottom`),"--n-font-weight-strong":_,"--n-tab-color-segment":v}}),$=r?u(`tabs`,L(()=>`${D.value[0]}${e.type[0]}`),Ae,e):void 0;return{mergedClsPrefix:n,mergedValue:P,renderedNames:new Set,segmentCapsuleElRef:Y,tabsPaneWrapperRef:G,tabsElRef:d,selfElRef:f,barElRef:p,addTabInstRef:_,xScrollInstRef:b,scrollWrapperElRef:h,addTabFixed:Z,tabWrapperStyle:R,handleNavResize:ye,mergedSize:D,handleScroll:De,handleTabsResize:Se,cssVars:r?void 0:Ae,themeClass:$?.themeClass,animationDirection:de,renderNameListRef:ue,yScrollElRef:S,handleSegmentResize:ke,onAnimationBeforeLeave:se,onAnimationEnter:ce,onAnimationAfterEnter:le,onRender:$?.onRender,startReachedRef:w,endReachedRef:T,isOverflow:we,handleButtonClick:he,mergedTheme:l,rtlEnabled:s,mergedPlacement:c,...Oe}},render(){let{mergedClsPrefix:e,type:t,mergedPlacement:r,addTabFixed:i,addable:a,mergedSize:o,renderNameListRef:c,onRender:l,paneWrapperClass:u,paneWrapperStyle:d,startReachedRef:f,endReachedRef:p,isOverflow:m,showScrollButton:g,handleButtonClick:v,mergedTheme:y,rtlEnabled:b,$slots:{default:x,prefix:S,suffix:C}}=this;l?.();let w=x?oe(x()).filter(e=>e.type.__TAB_PANE__===!0):[],T=x?oe(x()).filter(e=>e.type.__TAB__===!0):[],D=!T.length,O=t===`card`,k=t===`segment`,A=!O&&!k&&this.justifyContent;c.value=[];let j=()=>{let t=(P(),z(`div`,{style:F(this.tabWrapperStyle),class:s(`${e}-tabs-wrapper`)},[A?_(()=>null):(P(),z(`div`,{key:1,class:s(`${e}-tabs-scroll-padding`),style:F(r===`top`||r===`bottom`?{width:`${this.tabsPadding}px`}:{height:`${this.tabsPadding}px`})},null,6)),D?(P(),z(B,{key:2},[_(()=>w.map((e,t)=>(c.value.push(e.props.name),ze((P(),H($,V(e.props,{internalCreatedByPane:!0,internalLeftPadded:t!==0&&(!A||A===`center`||A===`start`||A===`end`)}),h(e.children?{default:e.children.tab}:void 0),1040,[`internalLeftPadded`]))))))],64)):(P(),z(B,{key:3},[_(()=>T.map((e,t)=>(c.value.push(e.props.name),ze(t!==0&&!A?Re(e):e))))],64)),!i&&a&&O?(P(),z(B,{key:4},[_(()=>Le(a,(D?w.length:T.length)!==0))],64)):_(()=>null),A?_(()=>null):(P(),z(`div`,{key:7,class:s(`${e}-tabs-scroll-padding`),style:F({width:`${this.tabsPadding}px`})},null,6)),O?_(()=>null):(P(),z(`div`,{key:9,ref:`barElRef`,class:s(`${e}-tabs-bar`)},null,2))],6));return P(),z(`div`,{ref:`tabsElRef`,class:s(`${e}-tabs-nav-scroll-content`)},[O&&a?(P(),H(E,{key:0,onResize:this.handleTabsResize},{default:()=>t},1032,[`onResize`])):(P(),z(B,{key:1},[_(()=>t)],64)),O?(P(),z(`div`,{key:2,class:s(`${e}-tabs-pad`)},null,2)):_(()=>null)],2)},M=k?`top`:r;return P(),z(`div`,{ref:`selfElRef`,class:s([`${e}-tabs`,this.themeClass,`${e}-tabs--${t}-type`,`${e}-tabs--${o}-size`,A&&`${e}-tabs--flex`,`${e}-tabs--${M}`,b&&`${e}-tabs--rtl`]),style:F(this.cssVars)},[R(`div`,{class:s([`${e}-tabs-nav--${t}-type`,`${e}-tabs-nav--${M}`,`${e}-tabs-nav`])},[_(()=>n(S,t=>t&&(P(),z(`div`,{class:s(`${e}-tabs-nav__prefix`)},[_(()=>t)],2)))),k?(P(),H(E,{key:0,onResize:this.handleSegmentResize},{default:()=>(P(),z(`div`,{class:s(`${e}-tabs-rail`),ref:`tabsElRef`},[R(`div`,{class:s(`${e}-tabs-capsule`),ref:`segmentCapsuleElRef`},[R(`div`,{class:s(`${e}-tabs-wrapper`)},[R(`div`,{class:s(`${e}-tabs-tab`)},null,2)],2)],2),D?(P(),z(B,{key:0},[_(()=>w.map((e,t)=>(c.value.push(e.props.name),P(),H($,V(e.props,{internalCreatedByPane:!0,internalLeftPadded:t!==0}),h(e.children?{default:e.children.tab}:void 0),1040,[`internalLeftPadded`]))))],64)):(P(),z(B,{key:1},[_(()=>T.map((e,t)=>(c.value.push(e.props.name),t===0?e:Re(e))))],64))],2))},1032,[`onResize`])):(P(),z(B,{key:1},[_(()=>g&&m&&(P(),H(Me,{mergedClsPrefix:e,type:`prev`,vertical:M===`left`||M===`right`,disabled:f,rtl:!!b,theme:y.peers.Button,themeOverrides:y.peerOverrides.Button,onClick:v},null,8,[`mergedClsPrefix`,`vertical`,`disabled`,`rtl`,`theme`,`themeOverrides`,`onClick`]))),(P(),H(E,{onResize:this.handleNavResize},{default:()=>(P(),z(`div`,{class:s(`${e}-tabs-nav-scroll-wrapper`),ref:`scrollWrapperElRef`},[[`top`,`bottom`].includes(M)?(P(),H(Q,{key:0,ref:`xScrollInstRef`,onScroll:this.handleScroll},{default:j},1032,[`onScroll`])):(P(),z(`div`,{key:1,class:s(`${e}-tabs-nav-y-scroll`),onScroll:this.handleScroll,ref:`yScrollElRef`},[_(()=>j())],42,[`onScroll`]))],2))},1032,[`onResize`])),_(()=>g&&m&&(P(),H(Me,{mergedClsPrefix:e,type:`next`,vertical:M===`left`||M===`right`,disabled:p,rtl:!!b,theme:y.peers.Button,themeOverrides:y.peerOverrides.Button,onClick:v},null,8,[`mergedClsPrefix`,`vertical`,`disabled`,`rtl`,`theme`,`themeOverrides`,`onClick`])))],64)),i&&a&&O?(P(),z(B,{key:2},[_(()=>Le(a,!0))],64)):_(()=>null),_(()=>n(C,t=>t&&(P(),z(`div`,{class:s(`${e}-tabs-nav__suffix`)},[_(()=>t)],2))))],2),_(()=>D&&(this.animated&&(M===`top`||M===`bottom`)?(P(),z(`div`,{key:1,ref:`tabsPaneWrapperRef`,style:F(d),class:s([`${e}-tabs-pane-wrapper`,u])},[_(()=>Ie(w,this.mergedValue,this.renderedNames,this.onAnimationBeforeLeave,this.onAnimationEnter,this.onAnimationAfterEnter,this.animationDirection))],6)):Ie(w,this.mergedValue,this.renderedNames)))],6)}});function Ie(e,t,n,r,i,a,o){let s=[];return e.forEach(e=>{let{name:r,displayDirective:i,"display-directive":a}=e.props,o=e=>i===e||a===e,c=t===r;if(e.key!==void 0&&(e.key=r),c||o(`show`)||o(`show:lazy`)&&n.has(r)){n.has(r)||n.add(r);let t=!o(`if`);s.push(t?te(e,[[W,c]]):e)}}),o?(P(),H(G,{name:`${o}-transition`,onBeforeLeave:r,onEnter:i,onAfterEnter:a},{default:()=>s},1032,[`name`,`onBeforeLeave`,`onEnter`,`onAfterEnter`])):s}function Le(e,t){return P(),H($,{ref:`addTabInstRef`,key:`__addable`,name:`__addable`,internalCreatedByPane:!0,internalAddable:!0,internalLeftPadded:t,disabled:typeof e==`object`&&e.disabled},null,8,[`internalLeftPadded`,`disabled`])}function Re(e){let t=ne(e);return t.props?t.props.internalLeftPadded=!0:t.props={internalLeftPadded:!0},t}function ze(e){return Array.isArray(e.dynamicProps)?e.dynamicProps.includes(`internalLeftPadded`)||e.dynamicProps.push(`internalLeftPadded`):e.dynamicProps=[`internalLeftPadded`],e}export{$ as n,Oe as r,Fe as t};