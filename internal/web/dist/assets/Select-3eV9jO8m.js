import{_ as e,a as t,at as n,c as r,ct as i,g as a,h as o,it as s,l as c,lt as l,r as u,st as d,ut as f,w as p,y as m}from"./resolve-slot-D0ESJhJD.js";import{n as h,t as g}from"./use-rtl-B41EZXdW.js";import{f as _,p as v,u as y,y as b}from"./light-D4diJaDS.js";import{a as x,n as S,r as C}from"./replaceable-CG1Qs4lR.js";import{t as w}from"./Popover-DfEILjfp.js";import{i as T,l as ee,o as E,t as D}from"./Scrollbar-D9NqNphl.js";import{E as O,F as k,H as te,M as ne,O as A,R as j,S as M,St as N,V as P,Wt as F,_t as I,b as L,d as R,f as z,h as B,i as V,it as re,j as H,ot as U,p as W,rt as G}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{a as K,t as q}from"./runtime-dom.esm-bundler-B3Pl3ttw.js";import{t as J}from"./happens-in-D4nrdV77.js";import{_ as ie,d as ae,f as Y,s as oe}from"./vue-i18n-D4LRfH1J.js";import{t as se}from"./use-locale-JKfGS3H1.js";import{t as X}from"./Empty-C9RVhu5n.js";import{a as Z,i as ce,n as le,o as Q,r as ue,t as de}from"./Follower-B00Pedqi.js";import{a as fe,i as pe,o as $}from"./icon-switch.cssr-o4i5UUsg.js";import{t as me}from"./use-merged-state-KQOIh2IK.js";import{t as he}from"./use-compitable-CgN8vuBj.js";import{t as ge}from"./VirtualList-DJo9wtN9.js";import{t as _e}from"./src-091rUp-g.js";import{t as ve}from"./Tag-CfwO544n.js";import{l as ye}from"./Button-DUNxlS0h.js";import{t as be}from"./Suffix-RH0NE_XF.js";import{t as xe}from"./focus-detector-ByCUTYWH.js";import{i as Se,t as Ce}from"./create-DH59f11n.js";function we(e){switch(typeof e){case`string`:return e||void 0;case`number`:return String(e);default:return}}function Te(e,t){t&&(j(()=>{let{value:n}=e;n&&T.registerHandler(n,t)}),G(e,(e,t)=>{t&&T.unregisterHandler(t)},{deep:!1}),k(()=>{let{value:t}=e;t&&T.unregisterHandler(t)}))}var Ee=M({name:`NBaseSelectGroupHeader`,props:{clsPrefix:{type:String,required:!0},tmNode:{type:Object,required:!0}},setup(){let{renderLabelRef:e,renderOptionRef:t,labelFieldRef:n,nodePropsRef:r}=A(Q);return{labelField:n,nodeProps:r,renderLabel:e,renderOption:t}},render(){let{clsPrefix:e,renderLabel:t,renderOption:n,nodeProps:r,tmNode:{rawNode:i}}=this,a=r?.(i),o=t?t(i,!1):v(i[this.labelField],i,!1),s=(P(),B(`div`,H(a,{class:[`${e}-base-select-group-header`,a?.class]}),[m(()=>o)],16));return i.render?i.render({node:s,option:i}):n?n({node:s,option:i,selected:!1}):s}}),De=M({name:`Checkmark`,render(){return(()=>{let e=o(`3c84eac8ae4e1f96`);return e[0]||(e[0]=z(`svg`,{xmlns:`http://www.w3.org/2000/svg`,viewBox:`0 0 16 16`},[z(`g`,{fill:`none`},[z(`path`,{d:`M14.046 3.486a.75.75 0 0 1-.032 1.06l-7.93 7.474a.85.85 0 0 1-1.188-.022l-2.68-2.72a.75.75 0 1 1 1.068-1.053l2.234 2.267l7.468-7.038a.75.75 0 0 1 1.06.032z`,fill:`currentColor`})])],-1))})()}}),Oe=[`onClick`,`onMouseenter`,`onMousemove`];function ke(e,t){return P(),W(q,{name:`fade-in-scale-up-transition`},{default:()=>e?(P(),W(S,{key:1,clsPrefix:t,class:a(`${t}-base-select-option__check`)},{default:()=>O(De)},1032,[`clsPrefix`,`class`])):null},1024)}var Ae=M({name:`NBaseSelectOption`,props:{clsPrefix:{type:String,required:!0},tmNode:{type:Object,required:!0}},setup(e){let{valueRef:t,pendingTmNodeRef:n,multipleRef:r,valueSetRef:i,renderLabelRef:a,renderOptionRef:o,labelFieldRef:s,valueFieldRef:c,showCheckmarkRef:l,nodePropsRef:u,handleOptionClick:d,handleOptionMouseEnter:f}=A(Q),p=$(()=>{let{value:t}=n;return t?e.tmNode.key===t.key:!1});function m(t){let{tmNode:n}=e;n.disabled||d(t,n)}function h(t){let{tmNode:n}=e;n.disabled||f(t,n)}function g(t){let{tmNode:n}=e,{value:r}=p;n.disabled||r||f(t,n)}return{multiple:r,isGrouped:$(()=>{let{tmNode:t}=e,{parent:n}=t;return n&&n.rawNode.type===`group`}),showCheckmark:l,nodeProps:u,isPending:p,isSelected:$(()=>{let{value:n}=t,{value:a}=r;if(n===null)return!1;let o=e.tmNode.rawNode[c.value];if(a){let{value:e}=i;return e.has(o)}return n===o}),labelField:s,renderLabel:a,renderOption:o,handleMouseMove:g,handleMouseEnter:h,handleClick:m}},render(){let{clsPrefix:e,tmNode:{rawNode:t},isSelected:n,isPending:r,isGrouped:i,showCheckmark:o,nodeProps:s,renderOption:c,renderLabel:l,handleClick:u,handleMouseEnter:d,handleMouseMove:f}=this,p=ke(n,e),h=l?[l(t,n),o&&p]:[v(t[this.labelField],t,n),o&&p],g=s?.(t),_=(P(),B(`div`,H(g,{class:[`${e}-base-select-option`,t.class,g?.class,{[`${e}-base-select-option--disabled`]:t.disabled,[`${e}-base-select-option--selected`]:n,[`${e}-base-select-option--grouped`]:i,[`${e}-base-select-option--pending`]:r,[`${e}-base-select-option--show-checkmark`]:o}],style:[g?.style||``,t.style||``],onClick:Y([u,g?.onClick]),onMouseenter:Y([d,g?.onMouseenter]),onMousemove:Y([f,g?.onMousemove])}),[z(`div`,{class:a(`${e}-base-select-option__content`)},[m(()=>h)],2)],16,Oe));return t.render?t.render({node:_,option:t,selected:n}):c?c({node:_,option:t,selected:n}):_}}),je=n(`base-select-menu`,`
 line-height: 1.5;
 outline: none;
 z-index: 0;
 position: relative;
 border-radius: var(--n-border-radius);
 transition:
 background-color .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier);
 background-color: var(--n-color);
`,[n(`scrollbar`,`
 max-height: var(--n-height);
 `),n(`virtual-list`,`
 max-height: var(--n-height);
 `),n(`base-select-option`,`
 min-height: var(--n-option-height);
 font-size: var(--n-option-font-size);
 display: flex;
 align-items: center;
 `,[d(`content`,`
 z-index: 1;
 white-space: nowrap;
 text-overflow: ellipsis;
 overflow: hidden;
 `)]),n(`base-select-group-header`,`
 min-height: var(--n-option-height);
 font-size: .93em;
 display: flex;
 align-items: center;
 `),n(`base-select-menu-option-wrapper`,`
 position: relative;
 width: 100%;
 `),d(`loading, empty`,`
 display: flex;
 padding: 12px 32px;
 flex: 1;
 justify-content: center;
 `),d(`loading`,`
 color: var(--n-loading-color);
 font-size: var(--n-loading-size);
 `),d(`header`,`
 padding: 8px var(--n-option-padding-left);
 font-size: var(--n-option-font-size);
 transition: 
 color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 border-bottom: 1px solid var(--n-action-divider-color);
 color: var(--n-action-text-color);
 `),d(`action`,`
 padding: 8px var(--n-option-padding-left);
 font-size: var(--n-option-font-size);
 transition: 
 color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 border-top: 1px solid var(--n-action-divider-color);
 color: var(--n-action-text-color);
 `),n(`base-select-group-header`,`
 position: relative;
 cursor: default;
 padding: var(--n-option-padding);
 color: var(--n-group-header-text-color);
 `),n(`base-select-option`,`
 cursor: pointer;
 position: relative;
 padding: var(--n-option-padding);
 transition:
 color .3s var(--n-bezier),
 opacity .3s var(--n-bezier);
 box-sizing: border-box;
 color: var(--n-option-text-color);
 opacity: 1;
 `,[i(`show-checkmark`,`
 padding-right: calc(var(--n-option-padding-right) + 20px);
 `),s(`&::before`,`
 content: "";
 position: absolute;
 left: 4px;
 right: 4px;
 top: 0;
 bottom: 0;
 border-radius: var(--n-border-radius);
 transition: background-color .3s var(--n-bezier);
 `),s(`&:active`,`
 color: var(--n-option-text-color-pressed);
 `),i(`grouped`,`
 padding-left: calc(var(--n-option-padding-left) * 1.5);
 `),i(`pending`,[s(`&::before`,`
 background-color: var(--n-option-color-pending);
 `)]),i(`selected`,`
 color: var(--n-option-text-color-active);
 `,[s(`&::before`,`
 background-color: var(--n-option-color-active);
 `),i(`pending`,[s(`&::before`,`
 background-color: var(--n-option-color-active-pending);
 `)])]),i(`disabled`,`
 cursor: not-allowed;
 `,[l(`selected`,`
 color: var(--n-option-text-color-disabled);
 `),i(`selected`,`
 opacity: var(--n-option-opacity-disabled);
 `)]),d(`check`,`
 font-size: 16px;
 position: absolute;
 right: calc(var(--n-option-padding-right) - 4px);
 top: calc(50% - 7px);
 color: var(--n-option-check-color);
 transition: color .3s var(--n-bezier);
 `,[_({enterScale:`0.5`})])])]),Me=[`tabindex`,`onFocusin`,`onFocusout`,`onKeyup`,`onKeydown`,`onMousedown`,`onMouseenter`,`onMouseleave`],Ne=M({name:`InternalSelectMenu`,props:{...r.props,clsPrefix:{type:String,required:!0},scrollable:{type:Boolean,default:!0},treeMate:{type:Object,required:!0},multiple:Boolean,size:{type:String,default:`medium`},value:{type:[String,Number,Array],default:null},autoPending:Boolean,virtualScroll:{type:Boolean,default:!0},show:{type:Boolean,default:!0},labelField:{type:String,default:`label`},valueField:{type:String,default:`value`},loading:Boolean,focusable:Boolean,renderLabel:Function,renderOption:Function,nodeProps:Function,showCheckmark:{type:Boolean,default:!0},onMousedown:Function,onScroll:Function,onFocus:Function,onBlur:Function,onKeyup:Function,onKeydown:Function,onTabOut:Function,onMouseenter:Function,onMouseleave:Function,onResize:Function,resetMenuOnOptionsChange:{type:Boolean,default:!0},inlineThemeDisabled:Boolean,scrollbarProps:Object,onToggle:Function},setup(e){let{mergedClsPrefixRef:t,mergedRtlRef:n,mergedComponentPropsRef:i}=p(e),a=g(`InternalSelectMenu`,n,t),o=r(`InternalSelectMenu`,`-internal-select-menu`,je,ie,e,N(e,`clsPrefix`)),s=I(null),l=I(null),u=I(null),d=R(()=>e.treeMate.getFlattenedNodes()),m=R(()=>Se(d.value)),h=I(null);function _(){let{treeMate:t}=e,n=null,{value:r}=e;r===null?n=t.getFirstAvailableNode():(n=e.multiple?t.getNode((r||[])[(r||[]).length-1]):t.getNode(r),(!n||n.disabled)&&(n=t.getFirstAvailableNode())),H(n||null)}function v(){let{value:t}=h;t&&!e.treeMate.getNode(t.key)&&(h.value=null)}let y;G(()=>e.show,t=>{t?y=G(()=>e.treeMate,()=>{e.resetMenuOnOptionsChange?(e.autoPending?_():v(),ne(U)):v()},{immediate:!0}):y?.()},{immediate:!0}),k(()=>{y?.()});let b=R(()=>C(o.value.self[f(`optionHeight`,e.size)])),S=R(()=>x(o.value.self[f(`padding`,e.size)])),w=R(()=>e.multiple&&Array.isArray(e.value)?new Set(e.value):new Set),T=R(()=>{let e=d.value;return e&&e.length===0}),ee=R(()=>i?.value?.Select?.renderEmpty);function E(t){let{onToggle:n}=e;n&&n(t)}function D(t){let{onScroll:n}=e;n&&n(t)}function O(e){u.value?.sync(),D(e)}function A(){u.value?.sync()}function M(){let{value:e}=h;return e||null}function P(e,t){t.disabled||H(t,!1)}function F(e,t){t.disabled||E(t)}function L(t){J(t,`action`)||e.onKeyup?.(t)}function z(t){J(t,`action`)||e.onKeydown?.(t)}function B(t){e.onMousedown?.(t),!e.focusable&&t.preventDefault()}function V(){let{value:e}=h;e&&H(e.getNext({loop:!0}),!0)}function re(){let{value:e}=h;e&&H(e.getPrev({loop:!0}),!0)}function H(e,t=!1){h.value=e,t&&U()}function U(){let t=h.value;if(!t)return;let n=m.value(t.key);n!==null&&(e.virtualScroll?l.value?.scrollTo({index:n}):u.value?.scrollTo({index:n,elSize:b.value}))}function W(t){s.value?.contains(t.target)&&e.onFocus?.(t)}function K(t){s.value?.contains(t.relatedTarget)||e.onBlur?.(t)}te(Q,{handleOptionMouseEnter:P,handleOptionClick:F,valueSetRef:w,pendingTmNodeRef:h,nodePropsRef:N(e,`nodeProps`),showCheckmarkRef:N(e,`showCheckmark`),multipleRef:N(e,`multiple`),valueRef:N(e,`value`),renderLabelRef:N(e,`renderLabel`),renderOptionRef:N(e,`renderOption`),labelFieldRef:N(e,`labelField`),valueFieldRef:N(e,`valueField`)}),te(Z,s),j(()=>{let{value:e}=u;e&&e.sync()});let q=R(()=>{let{size:t}=e,{common:{cubicBezierEaseInOut:n},self:{height:r,borderRadius:i,color:a,groupHeaderTextColor:s,actionDividerColor:c,optionTextColorPressed:l,optionTextColor:u,optionTextColorDisabled:d,optionTextColorActive:p,optionOpacityDisabled:m,optionCheckColor:h,actionTextColor:g,optionColorPending:_,optionColorActive:v,loadingColor:y,loadingSize:b,optionColorActivePending:S,[f(`optionFontSize`,t)]:C,[f(`optionHeight`,t)]:w,[f(`optionPadding`,t)]:T}}=o.value;return{"--n-height":r,"--n-action-divider-color":c,"--n-action-text-color":g,"--n-bezier":n,"--n-border-radius":i,"--n-color":a,"--n-option-font-size":C,"--n-group-header-text-color":s,"--n-option-check-color":h,"--n-option-color-pending":_,"--n-option-color-active":v,"--n-option-color-active-pending":S,"--n-option-height":w,"--n-option-opacity-disabled":m,"--n-option-text-color":u,"--n-option-text-color-active":p,"--n-option-text-color-disabled":d,"--n-option-text-color-pressed":l,"--n-option-padding":T,"--n-option-padding-left":x(T,`left`),"--n-option-padding-right":x(T,`right`),"--n-loading-color":y,"--n-loading-size":b}}),{inlineThemeDisabled:ae}=e,Y=ae?c(`internal-select-menu`,R(()=>e.size[0]),q,e):void 0,oe={selfRef:s,next:V,prev:re,getPendingTmNode:M};return Te(s,e.onResize),{mergedTheme:o,mergedClsPrefix:t,rtlEnabled:a,virtualListRef:l,scrollbarRef:u,itemSize:b,padding:S,flattenedNodes:d,empty:T,mergedRenderEmpty:ee,virtualListContainer(){let{value:e}=l;return e?.listElRef},virtualListContent(){let{value:e}=l;return e?.itemsElRef},doScroll:D,handleFocusin:W,handleFocusout:K,handleKeyUp:L,handleKeyDown:z,handleMouseDown:B,handleVirtualListResize:A,handleVirtualListScroll:O,cssVars:ae?void 0:q,themeClass:Y?.themeClass,onRender:Y?.onRender,...oe}},render(){let{$slots:e,virtualScroll:n,clsPrefix:r,mergedTheme:i,themeClass:o,onRender:s}=this;return s?.(),P(),B(`div`,{ref:`selfRef`,tabindex:this.focusable?0:-1,class:a([`${r}-base-select-menu`,`${r}-base-select-menu--${this.size}-size`,this.rtlEnabled&&`${r}-base-select-menu--rtl`,o,this.multiple&&`${r}-base-select-menu--multiple`]),style:F(this.cssVars),onFocusin:this.handleFocusin,onFocusout:this.handleFocusout,onKeyup:this.handleKeyUp,onKeydown:this.handleKeyDown,onMousedown:this.handleMouseDown,onMouseenter:this.onMouseenter,onMouseleave:this.onMouseleave},[m(()=>t(e.header,e=>e&&(P(),B(`div`,{class:a(`${r}-base-select-menu__header`),"data-header":!0,key:`header`},[m(()=>e)],2)))),this.loading?(P(),B(`div`,{key:0,class:a(`${r}-base-select-menu__loading`)},[(P(),W(ye,{clsPrefix:r,strokeWidth:20},null,8,[`clsPrefix`]))],2)):(P(),B(V,{key:1},[this.empty?(P(),B(`div`,{key:1,class:a(`${r}-base-select-menu__empty`),"data-empty":!0},[m(()=>u(e.empty,()=>[this.mergedRenderEmpty?.()||(P(),W(X,{theme:i.peers.Empty,themeOverrides:i.peerOverrides.Empty,size:this.size},null,8,[`theme`,`themeOverrides`,`size`]))]))],2)):(P(),W(D,H({key:0,ref:`scrollbarRef`,theme:i.peers.Scrollbar,themeOverrides:i.peerOverrides.Scrollbar,scrollable:this.scrollable,container:n?this.virtualListContainer:void 0,content:n?this.virtualListContent:void 0,onScroll:n?void 0:this.doScroll},this.scrollbarProps),{default:()=>n?(P(),W(ge,{key:1,ref:`virtualListRef`,class:a(`${r}-virtual-list`),items:this.flattenedNodes,itemSize:this.itemSize,showScrollbar:!1,paddingTop:this.padding.top,paddingBottom:this.padding.bottom,onResize:this.handleVirtualListResize,onScroll:this.handleVirtualListScroll,itemResizable:!0},{default:({item:e})=>e.isGroup?(P(),W(Ee,{key:e.key,clsPrefix:r,tmNode:e},null,8,[`clsPrefix`,`tmNode`])):e.ignored?null:(P(),W(Ae,{clsPrefix:r,key:e.key,tmNode:e},null,8,[`clsPrefix`,`tmNode`]))},1032,[`class`,`items`,`itemSize`,`paddingTop`,`paddingBottom`,`onResize`,`onScroll`])):(P(),B(`div`,{key:4,class:a(`${r}-base-select-menu-option-wrapper`),style:F({paddingTop:this.padding.top,paddingBottom:this.padding.bottom})},[m(()=>this.flattenedNodes.map(e=>e.isGroup?(P(),W(Ee,{key:e.key,clsPrefix:r,tmNode:e},null,8,[`clsPrefix`,`tmNode`])):(P(),W(Ae,{clsPrefix:r,key:e.key,tmNode:e},null,8,[`clsPrefix`,`tmNode`]))))],6))},1040,[`theme`,`themeOverrides`,`scrollable`,`container`,`content`,`onScroll`]))],64)),m(()=>t(e.action,e=>e&&[(P(),B(`div`,{class:a(`${r}-base-select-menu__action`),"data-action":!0,key:`action`},[m(()=>e)],2)),(P(),W(xe,{onFocus:this.onTabOut,key:`focus-detector`},null,8,[`onFocus`]))]))],46,Me)}});function Pe(e){return e.type===`group`}function Fe(e){return e.type===`ignored`}function Ie(e,t){try{return!!(1+t.toString().toLowerCase().indexOf(e.trim().toLowerCase()))}catch{return!1}}function Le(e,t){return{getIsGroup:Pe,getIgnored:Fe,getKey(t){return Pe(t)?t.name||t.key||`key-required`:t[e]},getChildren(e){return e[t]}}}function Re(e,t,n,r){if(!t)return e;function i(e){if(!Array.isArray(e))return[];let a=[];for(let o of e)if(Pe(o)){let e=i(o[r]);e.length&&a.push(Object.assign({},o,{[r]:e}))}else if(Fe(o))continue;else t(n,o)&&a.push(o);return a}return i(e)}function ze(e,t,n){let r=new Map;return e.forEach(e=>{Pe(e)?e[n].forEach(e=>{r.set(e[t],e)}):r.set(e[t],e)}),r}var Be=s([n(`base-selection`,`
 --n-padding-single: var(--n-padding-single-top) var(--n-padding-single-right) var(--n-padding-single-bottom) var(--n-padding-single-left);
 --n-padding-multiple: var(--n-padding-multiple-top) var(--n-padding-multiple-right) var(--n-padding-multiple-bottom) var(--n-padding-multiple-left);
 position: relative;
 z-index: auto;
 box-shadow: none;
 width: 100%;
 max-width: 100%;
 display: inline-block;
 vertical-align: bottom;
 border-radius: var(--n-border-radius);
 min-height: var(--n-height);
 line-height: 1.5;
 font-size: var(--n-font-size);
 `,[n(`base-loading`,`
 color: var(--n-loading-color);
 `),n(`base-selection-tags`,`min-height: var(--n-height);`),d(`border, state-border`,`
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 pointer-events: none;
 border: var(--n-border);
 border-radius: inherit;
 transition:
 box-shadow .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `),d(`state-border`,`
 z-index: 1;
 border-color: #0000;
 `),n(`base-suffix`,`
 cursor: pointer;
 position: absolute;
 top: 50%;
 transform: translateY(-50%);
 right: 10px;
 `,[d(`arrow`,`
 font-size: var(--n-arrow-size);
 color: var(--n-arrow-color);
 transition: color .3s var(--n-bezier);
 `)]),n(`base-selection-overlay`,`
 display: flex;
 align-items: center;
 white-space: nowrap;
 pointer-events: none;
 position: absolute;
 top: 0;
 right: 0;
 bottom: 0;
 left: 0;
 padding: var(--n-padding-single);
 transition: color .3s var(--n-bezier);
 `,[d(`wrapper`,`
 flex-basis: 0;
 flex-grow: 1;
 overflow: hidden;
 text-overflow: ellipsis;
 `)]),n(`base-selection-placeholder`,`
 color: var(--n-placeholder-color);
 `,[d(`inner`,`
 max-width: 100%;
 overflow: hidden;
 `)]),n(`base-selection-tags`,`
 cursor: pointer;
 outline: none;
 box-sizing: border-box;
 position: relative;
 z-index: auto;
 display: flex;
 padding: var(--n-padding-multiple);
 flex-wrap: wrap;
 align-items: center;
 width: 100%;
 vertical-align: bottom;
 background-color: var(--n-color);
 border-radius: inherit;
 transition:
 color .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 `),n(`base-selection-label`,`
 height: var(--n-height);
 display: inline-flex;
 width: 100%;
 vertical-align: bottom;
 cursor: pointer;
 outline: none;
 z-index: auto;
 box-sizing: border-box;
 position: relative;
 transition:
 color .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 border-radius: inherit;
 background-color: var(--n-color);
 align-items: center;
 `,[n(`base-selection-input`,`
 font-size: inherit;
 line-height: inherit;
 outline: none;
 cursor: pointer;
 box-sizing: border-box;
 border:none;
 width: 100%;
 padding: var(--n-padding-single);
 background-color: #0000;
 color: var(--n-text-color);
 transition: color .3s var(--n-bezier);
 caret-color: var(--n-caret-color);
 `,[d(`content`,`
 text-overflow: ellipsis;
 overflow: hidden;
 white-space: nowrap; 
 `)]),d(`render-label`,`
 color: var(--n-text-color);
 `)]),l(`disabled`,[s(`&:hover`,[d(`state-border`,`
 box-shadow: var(--n-box-shadow-hover);
 border: var(--n-border-hover);
 `)]),i(`focus`,[d(`state-border`,`
 box-shadow: var(--n-box-shadow-focus);
 border: var(--n-border-focus);
 `)]),i(`active`,[d(`state-border`,`
 box-shadow: var(--n-box-shadow-active);
 border: var(--n-border-active);
 `),n(`base-selection-label`,`background-color: var(--n-color-active);`),n(`base-selection-tags`,`background-color: var(--n-color-active);`)])]),i(`disabled`,`cursor: not-allowed;`,[d(`arrow`,`
 color: var(--n-arrow-color-disabled);
 `),n(`base-selection-label`,`
 cursor: not-allowed;
 background-color: var(--n-color-disabled);
 `,[n(`base-selection-input`,`
 cursor: not-allowed;
 color: var(--n-text-color-disabled);
 `),d(`render-label`,`
 color: var(--n-text-color-disabled);
 `)]),n(`base-selection-tags`,`
 cursor: not-allowed;
 background-color: var(--n-color-disabled);
 `),n(`base-selection-placeholder`,`
 cursor: not-allowed;
 color: var(--n-placeholder-color-disabled);
 `)]),n(`base-selection-input-tag`,`
 height: calc(var(--n-height) - 6px);
 line-height: calc(var(--n-height) - 6px);
 outline: none;
 display: none;
 position: relative;
 margin-bottom: 3px;
 max-width: 100%;
 vertical-align: bottom;
 `,[d(`input`,`
 font-size: inherit;
 font-family: inherit;
 min-width: 1px;
 padding: 0;
 background-color: #0000;
 outline: none;
 border: none;
 max-width: 100%;
 overflow: hidden;
 width: 1em;
 line-height: inherit;
 cursor: pointer;
 color: var(--n-text-color);
 caret-color: var(--n-caret-color);
 `),d(`mirror`,`
 position: absolute;
 left: 0;
 top: 0;
 white-space: pre;
 visibility: hidden;
 user-select: none;
 -webkit-user-select: none;
 opacity: 0;
 `)]),[`warning`,`error`].map(e=>i(`${e}-status`,[d(`state-border`,`border: var(--n-border-${e});`),l(`disabled`,[s(`&:hover`,[d(`state-border`,`
 box-shadow: var(--n-box-shadow-hover-${e});
 border: var(--n-border-hover-${e});
 `)]),i(`active`,[d(`state-border`,`
 box-shadow: var(--n-box-shadow-active-${e});
 border: var(--n-border-active-${e});
 `),n(`base-selection-label`,`background-color: var(--n-color-active-${e});`),n(`base-selection-tags`,`background-color: var(--n-color-active-${e});`)]),i(`focus`,[d(`state-border`,`
 box-shadow: var(--n-box-shadow-focus-${e});
 border: var(--n-border-focus-${e});
 `)])])]))]),n(`base-selection-popover`,`
 margin-bottom: -3px;
 display: flex;
 flex-wrap: wrap;
 margin-right: -8px;
 `),n(`base-selection-tag-wrapper`,`
 max-width: 100%;
 display: inline-flex;
 padding: 0 7px 3px 0;
 `,[s(`&:last-child`,`padding-right: 0;`),n(`tag`,`
 font-size: 14px;
 max-width: 100%;
 `,[d(`content`,`
 line-height: 1.25;
 text-overflow: ellipsis;
 overflow: hidden;
 `)])])]),Ve=[`disabled`,`value`,`autofocus`,`onBlur`,`onFocus`,`onKeydown`,`onInput`,`onCompositionstart`,`onCompositionend`],He=[`tabindex`],Ue=[`title`],We=[`value`,`readonly`,`disabled`,`autofocus`,`onFocus`,`onBlur`,`onInput`,`onCompositionstart`,`onCompositionend`],Ge=[`tabindex`],Ke=[`onClick`,`onMouseenter`,`onMouseleave`,`onKeydown`,`onFocusin`,`onFocusout`,`onMousedown`],qe=M({name:`InternalSelection`,props:{...r.props,clsPrefix:{type:String,required:!0},bordered:{type:Boolean,default:void 0},active:Boolean,pattern:{type:String,default:``},placeholder:String,selectedOption:{type:Object,default:null},selectedOptions:{type:Array,default:null},labelField:{type:String,default:`label`},valueField:{type:String,default:`value`},multiple:Boolean,filterable:Boolean,clearable:Boolean,disabled:Boolean,size:{type:String,default:`medium`},loading:Boolean,autofocus:Boolean,showArrow:{type:Boolean,default:!0},inputProps:Object,focused:Boolean,renderTag:Function,onKeydown:Function,onClick:Function,onBlur:Function,onFocus:Function,onDeleteOption:Function,maxTagCount:[String,Number],ellipsisTagPopoverProps:Object,onClear:Function,onPatternInput:Function,onPatternFocus:Function,onPatternBlur:Function,renderLabel:Function,status:String,inlineThemeDisabled:Boolean,ignoreComposition:{type:Boolean,default:!0},onResize:Function},setup(e){let{mergedClsPrefixRef:t,mergedRtlRef:n}=p(e),i=g(`InternalSelection`,n,t),a=I(null),o=I(null),s=I(null),l=I(null),u=I(null),d=I(null),m=I(null),h=I(null),_=I(null),y=I(null),b=I(!1),S=I(!1),C=I(!1),w=r(`InternalSelection`,`-internal-selection`,Be,ae,e,N(e,`clsPrefix`)),T=R(()=>e.clearable&&!e.disabled&&(C.value||e.active)),ee=R(()=>e.selectedOption?e.renderTag?e.renderTag({option:e.selectedOption,handleClose:()=>{}}):e.renderLabel?e.renderLabel(e.selectedOption,!0):v(e.selectedOption[e.labelField],e.selectedOption,!0):e.placeholder),E=R(()=>{let t=e.selectedOption;if(t)return t[e.labelField]}),D=R(()=>e.multiple?!!(Array.isArray(e.selectedOptions)&&e.selectedOptions.length):e.selectedOption!==null);function O(){let{value:t}=a;if(t){let{value:n}=o;n&&(n.style.width=`${t.offsetWidth}px`,e.maxTagCount!==`responsive`&&_.value?.sync({showAllItemsBeforeCalculate:!1}))}}function k(){let{value:e}=y;e&&(e.style.display=`none`)}function te(){let{value:e}=y;e&&(e.style.display=`inline-block`)}G(N(e,`active`),e=>{e||k()}),G(N(e,`pattern`),()=>{e.multiple&&ne(O)});function A(t){let{onFocus:n}=e;n&&n(t)}function M(t){let{onBlur:n}=e;n&&n(t)}function P(t){let{onDeleteOption:n}=e;n&&n(t)}function F(t){let{onClear:n}=e;n&&n(t)}function L(t){let{onPatternInput:n}=e;n&&n(t)}function z(e){e.relatedTarget&&s.value?.contains(e.relatedTarget)||A(e)}function B(e){s.value?.contains(e.relatedTarget)||M(e)}function V(e){F(e)}function H(){C.value=!0}function U(){C.value=!1}function W(t){e.active&&e.filterable&&t.target!==o.value&&t.preventDefault()}function K(e){P(e)}let q=I(!1);function J(t){if(t.key===`Backspace`&&!q.value&&!e.pattern.length){let{selectedOptions:t}=e;t?.length&&K(t[t.length-1])}}let ie=null;function Y(t){let{value:n}=a;n&&(n.textContent=t.target.value,O()),e.ignoreComposition&&q.value?ie=t:L(t)}function oe(){q.value=!0}function se(){q.value=!1,e.ignoreComposition&&L(ie),ie=null}function X(t){S.value=!0,e.onPatternFocus?.(t)}function Z(t){S.value=!1,e.onPatternBlur?.(t)}function ce(){if(e.filterable)S.value=!1,d.value?.blur(),o.value?.blur();else if(e.multiple){let{value:e}=l;e?.blur()}else{let{value:e}=u;e?.blur()}}function le(){e.filterable?(S.value=!1,d.value?.focus()):e.multiple?l.value?.focus():u.value?.focus()}function Q(){let{value:e}=o;e&&(te(),e.focus())}function ue(){let{value:e}=o;e&&e.blur()}function de(e){let{value:t}=m;t&&t.setTextContent(`+${e}`)}function fe(){let{value:e}=h;return e}function pe(){return o.value}let $=null;function me(){$!==null&&window.clearTimeout($)}function he(){e.active||(me(),$=window.setTimeout(()=>{D.value&&(b.value=!0)},100))}function ge(){me()}function _e(e){e||(me(),b.value=!1)}G(D,e=>{e||(b.value=!1)}),j(()=>{re(()=>{let t=d.value;t&&(e.disabled?t.removeAttribute(`tabindex`):t.tabIndex=S.value?-1:0)})}),Te(s,e.onResize);let{inlineThemeDisabled:ve}=e,ye=R(()=>{let{size:t}=e,{common:{cubicBezierEaseInOut:n},self:{fontWeight:r,borderRadius:i,color:a,placeholderColor:o,textColor:s,paddingSingle:c,paddingMultiple:l,caretColor:u,colorDisabled:d,textColorDisabled:p,placeholderColorDisabled:m,colorActive:h,boxShadowFocus:g,boxShadowActive:_,boxShadowHover:v,border:y,borderFocus:b,borderHover:S,borderActive:C,arrowColor:T,arrowColorDisabled:ee,loadingColor:E,colorActiveWarning:D,boxShadowFocusWarning:O,boxShadowActiveWarning:k,boxShadowHoverWarning:te,borderWarning:ne,borderFocusWarning:A,borderHoverWarning:j,borderActiveWarning:M,colorActiveError:N,boxShadowFocusError:P,boxShadowActiveError:F,boxShadowHoverError:I,borderError:L,borderFocusError:R,borderHoverError:z,borderActiveError:B,clearColor:V,clearColorHover:re,clearColorPressed:H,clearSize:U,arrowSize:W,[f(`height`,t)]:G,[f(`fontSize`,t)]:K}}=w.value,q=x(c),J=x(l);return{"--n-bezier":n,"--n-border":y,"--n-border-active":C,"--n-border-focus":b,"--n-border-hover":S,"--n-border-radius":i,"--n-box-shadow-active":_,"--n-box-shadow-focus":g,"--n-box-shadow-hover":v,"--n-caret-color":u,"--n-color":a,"--n-color-active":h,"--n-color-disabled":d,"--n-font-size":K,"--n-height":G,"--n-padding-single-top":q.top,"--n-padding-multiple-top":J.top,"--n-padding-single-right":q.right,"--n-padding-multiple-right":J.right,"--n-padding-single-left":q.left,"--n-padding-multiple-left":J.left,"--n-padding-single-bottom":q.bottom,"--n-padding-multiple-bottom":J.bottom,"--n-placeholder-color":o,"--n-placeholder-color-disabled":m,"--n-text-color":s,"--n-text-color-disabled":p,"--n-arrow-color":T,"--n-arrow-color-disabled":ee,"--n-loading-color":E,"--n-color-active-warning":D,"--n-box-shadow-focus-warning":O,"--n-box-shadow-active-warning":k,"--n-box-shadow-hover-warning":te,"--n-border-warning":ne,"--n-border-focus-warning":A,"--n-border-hover-warning":j,"--n-border-active-warning":M,"--n-color-active-error":N,"--n-box-shadow-focus-error":P,"--n-box-shadow-active-error":F,"--n-box-shadow-hover-error":I,"--n-border-error":L,"--n-border-focus-error":R,"--n-border-hover-error":z,"--n-border-active-error":B,"--n-clear-size":U,"--n-clear-color":V,"--n-clear-color-hover":re,"--n-clear-color-pressed":H,"--n-arrow-size":W,"--n-font-weight":r}}),be=ve?c(`internal-selection`,R(()=>e.size[0]),ye,e):void 0;return{mergedTheme:w,mergedClearable:T,mergedClsPrefix:t,rtlEnabled:i,patternInputFocused:S,filterablePlaceholder:ee,label:E,selected:D,showTagsPanel:b,isComposing:q,counterRef:m,counterWrapperRef:h,patternInputMirrorRef:a,patternInputRef:o,selfRef:s,multipleElRef:l,singleElRef:u,patternInputWrapperRef:d,overflowRef:_,inputTagElRef:y,handleMouseDown:W,handleFocusin:z,handleClear:V,handleMouseEnter:H,handleMouseLeave:U,handleDeleteOption:K,handlePatternKeyDown:J,handlePatternInputInput:Y,handlePatternInputBlur:Z,handlePatternInputFocus:X,handleMouseEnterCounter:he,handleMouseLeaveCounter:ge,handleFocusout:B,handleCompositionEnd:se,handleCompositionStart:oe,onPopoverUpdateShow:_e,focus:le,focusInput:Q,blur:ce,blurInput:ue,updateCounter:de,getCounter:fe,getTail:pe,renderLabel:e.renderLabel,cssVars:ve?void 0:ye,themeClass:be?.themeClass,onRender:be?.onRender}},render(){let{status:e,multiple:t,size:n,disabled:r,filterable:i,maxTagCount:o,bordered:s,clsPrefix:c,ellipsisTagPopoverProps:l,onRender:u,renderTag:d,renderLabel:f}=this;u?.();let p=o===`responsive`,h=typeof o==`number`,g=p||h,_=(P(),W(E,null,{default:()=>(P(),W(be,{clsPrefix:c,loading:this.loading,showArrow:this.showArrow,showClear:this.mergedClearable&&this.selected,onClear:this.handleClear},{default:()=>this.$slots.arrow?.()},1032,[`clsPrefix`,`loading`,`showArrow`,`showClear`,`onClear`]))},1024)),y;if(t){let{labelField:e}=this,t=t=>(P(),B(`div`,{class:a(`${c}-base-selection-tag-wrapper`),key:t.value},[d?(P(),B(V,{key:0},[m(()=>d({option:t,handleClose:()=>{this.handleDeleteOption(t)}}))],64)):(P(),W(ve,{key:1,size:n,closable:!t.disabled,disabled:r,onClose:()=>{this.handleDeleteOption(t)},internalCloseIsButtonTag:!1,internalCloseFocusable:!1},{default:()=>f?f(t,!0):v(t[e],t,!0)},1032,[`size`,`closable`,`disabled`,`onClose`]))],2)),s=()=>(h?this.selectedOptions.slice(0,o):this.selectedOptions).map(t),u=i?(P(),B(`div`,{class:a(`${c}-base-selection-input-tag`),ref:`inputTagElRef`,key:`__input-tag__`},[z(`input`,H(this.inputProps,{ref:`patternInputRef`,tabindex:-1,disabled:r,value:this.pattern,autofocus:this.autofocus,class:`${c}-base-selection-input-tag__input`,onBlur:this.handlePatternInputBlur,onFocus:this.handlePatternInputFocus,onKeydown:this.handlePatternKeyDown,onInput:this.handlePatternInputInput,onCompositionstart:this.handleCompositionStart,onCompositionend:this.handleCompositionEnd}),null,16,Ve),z(`span`,{ref:`patternInputMirrorRef`,class:a(`${c}-base-selection-input-tag__mirror`)},[m(()=>this.pattern)],2)],2)):null,b=p?()=>(P(),B(`div`,{class:a(`${c}-base-selection-tag-wrapper`),ref:`counterWrapperRef`},[(P(),W(ve,{size:n,ref:`counterRef`,onMouseenter:this.handleMouseEnterCounter,onMouseleave:this.handleMouseLeaveCounter,disabled:r},null,8,[`size`,`onMouseenter`,`onMouseleave`,`disabled`]))],2)):void 0,x;if(h){let e=this.selectedOptions.length-o;e>0&&(x=(t=>(P(),B(`div`,{class:a(`${c}-base-selection-tag-wrapper`),key:`__counter__`},[(P(),W(ve,{size:n,ref:`counterRef`,onMouseenter:this.handleMouseEnterCounter,disabled:r},{default:()=>`+${e}`},1032,[`size`,`onMouseenter`,`disabled`]))],2)))(x))}let S=p?i?(P(),W(_e,{key:3,ref:`overflowRef`,updateCounter:this.updateCounter,getCounter:this.getCounter,getTail:this.getTail,style:{width:`100%`,display:`flex`,overflow:`hidden`}},{default:s,counter:b,tail:()=>u},1032,[`updateCounter`,`getCounter`,`getTail`])):(P(),W(_e,{key:4,ref:`overflowRef`,updateCounter:this.updateCounter,getCounter:this.getCounter,style:{width:`100%`,display:`flex`,overflow:`hidden`}},{default:s,counter:b},1032,[`updateCounter`,`getCounter`])):h&&x?s().concat(x):s(),C=g?()=>(P(),B(`div`,{class:a(`${c}-base-selection-popover`)},[p?(P(),B(V,{key:0},[m(()=>s())],64)):(P(),B(V,{key:1},[m(()=>this.selectedOptions.map(t))],64))],2)):void 0,T=g?{show:this.showTagsPanel,trigger:`hover`,overlap:!0,placement:`top`,width:`trigger`,onUpdateShow:this.onPopoverUpdateShow,theme:this.mergedTheme.peers.Popover,themeOverrides:this.mergedTheme.peerOverrides.Popover,...l}:null,ee=!this.selected&&(!this.active||!this.pattern&&!this.isComposing)?(P(),B(`div`,{key:5,class:a(`${c}-base-selection-placeholder ${c}-base-selection-overlay`)},[z(`div`,{class:a(`${c}-base-selection-placeholder__inner`)},[m(()=>this.placeholder)],2)],2)):null,E=i?(P(),B(`div`,{key:6,ref:`patternInputWrapperRef`,class:a(`${c}-base-selection-tags`)},[m(()=>S),p?m(()=>null):(P(),B(V,{key:1},[m(()=>u)],64)),m(()=>_)],2)):(P(),B(`div`,{key:7,ref:`multipleElRef`,class:a(`${c}-base-selection-tags`),tabindex:r?void 0:0},[m(()=>S),m(()=>_)],10,He));y=(e=>(P(),B(V,{key:8},[g?(P(),W(w,H({key:0},T,{scrollable:!0,style:`max-height: calc(var(--v-target-height) * 6.6);`}),{trigger:()=>E,default:C},1040)):(P(),B(V,{key:1},[m(()=>E)],64)),m(()=>ee)],64)))(y)}else if(i){let e=this.pattern||this.isComposing,t=this.active?!e:!this.selected,n=!this.active&&this.selected;y=(e=>(P(),B(`div`,{key:9,ref:`patternInputWrapperRef`,class:a(`${c}-base-selection-label`),title:this.patternInputFocused?void 0:we(this.label)},[z(`input`,H(this.inputProps,{ref:`patternInputRef`,class:`${c}-base-selection-input`,value:this.active?this.pattern:``,placeholder:``,readonly:r,disabled:r,tabindex:-1,autofocus:this.autofocus,onFocus:this.handlePatternInputFocus,onBlur:this.handlePatternInputBlur,onInput:this.handlePatternInputInput,onCompositionstart:this.handleCompositionStart,onCompositionend:this.handleCompositionEnd}),null,16,We),n?(P(),B(`div`,{class:a(`${c}-base-selection-label__render-label ${c}-base-selection-overlay`),key:`input`},[z(`div`,{class:a(`${c}-base-selection-overlay__wrapper`)},[d?(P(),B(V,{key:0},[m(()=>d({option:this.selectedOption,handleClose:()=>{}}))],64)):(P(),B(V,{key:1},[f?(P(),B(V,{key:0},[m(()=>f(this.selectedOption,!0))],64)):(P(),B(V,{key:1},[m(()=>v(this.label,this.selectedOption,!0))],64))],64))],2)],2)):m(()=>null),t?(P(),B(`div`,{class:a(`${c}-base-selection-placeholder ${c}-base-selection-overlay`),key:`placeholder`},[z(`div`,{class:a(`${c}-base-selection-overlay__wrapper`)},[m(()=>this.filterablePlaceholder)],2)],2)):m(()=>null),m(()=>_)],10,Ue)))(y)}else y=(e=>(P(),B(`div`,{key:10,ref:`singleElRef`,class:a(`${c}-base-selection-label`),tabindex:this.disabled?void 0:0},[this.label===void 0?(P(),B(`div`,{class:a(`${c}-base-selection-placeholder ${c}-base-selection-overlay`),key:`placeholder`},[z(`div`,{class:a(`${c}-base-selection-placeholder__inner`)},[m(()=>this.placeholder)],2)],2)):(P(),B(`div`,{class:a(`${c}-base-selection-input`),title:we(this.label),key:`input`},[z(`div`,{class:a(`${c}-base-selection-input__content`)},[d?(P(),B(V,{key:0},[m(()=>d({option:this.selectedOption,handleClose:()=>{}}))],64)):(P(),B(V,{key:1},[f?(P(),B(V,{key:0},[m(()=>f(this.selectedOption,!0))],64)):(P(),B(V,{key:1},[m(()=>v(this.label,this.selectedOption,!0))],64))],64))],2)],10,[`title`])),m(()=>_)],10,Ge)))(y);return P(),B(`div`,{ref:`selfRef`,class:a([`${c}-base-selection`,this.rtlEnabled&&`${c}-base-selection--rtl`,this.themeClass,e&&`${c}-base-selection--${e}-status`,{[`${c}-base-selection--active`]:this.active,[`${c}-base-selection--selected`]:this.selected||this.active&&this.pattern,[`${c}-base-selection--disabled`]:this.disabled,[`${c}-base-selection--multiple`]:this.multiple,[`${c}-base-selection--focus`]:this.focused}]),style:F(this.cssVars),onClick:this.onClick,onMouseenter:this.handleMouseEnter,onMouseleave:this.handleMouseLeave,onKeydown:this.onKeydown,onFocusin:this.handleFocusin,onFocusout:this.handleFocusout,onMousedown:this.handleMouseDown},[m(()=>y),s?(P(),B(`div`,{key:0,class:a(`${c}-base-selection__border`)},null,2)):m(()=>null),s?(P(),B(`div`,{key:2,class:a(`${c}-base-selection__state-border`)},null,2)):m(()=>null)],46,Ke)}}),Je=s([n(`select`,`
 z-index: auto;
 outline: none;
 width: 100%;
 position: relative;
 font-weight: var(--n-font-weight);
 `),n(`select-menu`,`
 margin: 4px 0;
 box-shadow: var(--n-menu-box-shadow);
 `,[_({originalTransition:`background-color .3s var(--n-bezier), box-shadow .3s var(--n-bezier)`})])]),Ye={...r.props,to:ce.propTo,bordered:{type:Boolean,default:void 0},clearable:Boolean,clearCreatedOptionsOnClear:{type:Boolean,default:!0},clearFilterAfterSelect:{type:Boolean,default:!0},options:{type:Array,default:()=>[]},defaultValue:{type:[String,Number,Array],default:null},keyboard:{type:Boolean,default:!0},value:[String,Number,Array],placeholder:String,menuProps:Object,multiple:Boolean,size:String,menuSize:{type:String},filterable:Boolean,disabled:{type:Boolean,default:void 0},remote:Boolean,loading:Boolean,filter:Function,placement:{type:String,default:`bottom-start`},widthMode:{type:String,default:`trigger`},tag:Boolean,onCreate:Function,fallbackOption:{type:[Function,Boolean],default:void 0},show:{type:Boolean,default:void 0},showArrow:{type:Boolean,default:!0},maxTagCount:[Number,String],ellipsisTagPopoverProps:Object,consistentMenuWidth:{type:Boolean,default:!0},virtualScroll:{type:Boolean,default:!0},labelField:{type:String,default:`label`},valueField:{type:String,default:`value`},childrenField:{type:String,default:`children`},renderLabel:Function,renderOption:Function,renderTag:Function,"onUpdate:value":[Function,Array],inputProps:Object,nodeProps:Function,ignoreComposition:{type:Boolean,default:!0},showOnFocus:Boolean,onUpdateValue:[Function,Array],onBlur:[Function,Array],onClear:[Function,Array],onFocus:[Function,Array],onScroll:[Function,Array],onSearch:[Function,Array],onUpdateShow:[Function,Array],"onUpdate:show":[Function,Array],displayDirective:{type:String,default:`show`},resetMenuOnOptionsChange:{type:Boolean,default:!0},status:String,showCheckmark:{type:Boolean,default:!0},scrollbarProps:Object,onChange:[Function,Array],items:Array},Xe=M({name:`Select`,props:Ye,slots:Object,setup(e){let{mergedClsPrefixRef:t,mergedBorderedRef:n,namespaceRef:i,inlineThemeDisabled:a,mergedComponentPropsRef:o}=p(e),s=r(`Select`,`-select`,Je,oe,e,t),l=I(e.defaultValue),u=N(e,`value`),d=me(u,l),f=I(!1),m=I(``),g=he(e,[`items`,`options`]),_=I([]),v=I([]),b=R(()=>v.value.concat(_.value).concat(g.value)),x=R(()=>{let{filter:t}=e;if(t)return t;let{labelField:n,valueField:r}=e;return(e,t)=>{if(!t)return!1;let i=t[n];if(typeof i==`string`)return Ie(e,i);let a=t[r];return typeof a==`string`?Ie(e,a):typeof a==`number`&&Ie(e,String(a))}}),S=R(()=>{if(e.remote)return g.value;{let{value:t}=b,{value:n}=m;return!n.length||!e.filterable?t:Re(t,x.value,n,e.childrenField)}}),C=R(()=>{let{valueField:t,childrenField:n}=e,r=Le(t,n);return Ce(S.value,r)}),w=R(()=>ze(b.value,e.valueField,e.childrenField)),T=I(!1),E=me(N(e,`show`),T),D=I(null),O=I(null),k=I(null),{localeRef:te}=se(`Select`),ne=R(()=>e.placeholder??te.value.placeholder),A=[],j=I(new Map),M=R(()=>{let{fallbackOption:t}=e;if(t===void 0){let{labelField:t,valueField:n}=e;return e=>({[t]:String(e),[n]:e})}return t===!1?!1:e=>Object.assign(t(e),{value:e})});function P(t){let n=e.remote,{value:r}=j,{value:i}=w,{value:a}=M,o=[];return t.forEach(e=>{if(i.has(e))o.push(i.get(e));else if(n&&r.has(e))o.push(r.get(e));else if(a){let t=a(e);t&&o.push(t)}}),o}let F=R(()=>{if(e.multiple){let{value:e}=d;return Array.isArray(e)?P(e):[]}return null}),L=R(()=>{let{value:t}=d;return!e.multiple&&!Array.isArray(t)?t===null?null:P([t])[0]||null:null}),z=pe(e,{mergedSize:t=>{let{size:n}=e;if(n)return n;let{mergedSize:r}=t||{};return r?.value?r.value:o?.value?.Select?.size||`medium`}}),{mergedSizeRef:B,mergedDisabledRef:V,mergedStatusRef:re}=z;function H(t,n){let{onChange:r,"onUpdate:value":i,onUpdateValue:a}=e,{nTriggerFormChange:o,nTriggerFormInput:s}=z;r&&h(r,t,n),a&&h(a,t,n),i&&h(i,t,n),l.value=t,o(),s()}function U(t){let{onBlur:n}=e,{nTriggerFormBlur:r}=z;n&&h(n,t),r()}function W(){let{onClear:t}=e;t&&h(t)}function K(t){let{onFocus:n,showOnFocus:r}=e,{nTriggerFormFocus:i}=z;n&&h(n,t),i(),r&&X()}function q(t){let{onSearch:n}=e;n&&h(n,t)}function ie(t){let{onScroll:n}=e;n&&h(n,t)}function ae(){let{remote:t,multiple:n}=e;if(t){let{value:t}=j;if(n){let{valueField:n}=e;F.value?.forEach(e=>{t.set(e[n],e)})}else{let n=L.value;n&&t.set(n[e.valueField],n)}}}function Y(t){let{onUpdateShow:n,"onUpdate:show":r}=e;n&&h(n,t),r&&h(r,t),T.value=t}function X(){V.value||(Y(!0),T.value=!0,e.filterable&&Ne())}function Z(){Y(!1)}function le(){m.value=``,v.value=A}let Q=I(!1);function ue(){e.filterable&&(Q.value=!0)}function de(){e.filterable&&(Q.value=!1,E.value||le())}function $(){V.value||(E.value?e.filterable?Ne():Z():X())}function ge(e){k.value?.selfRef?.contains(e.relatedTarget)||(f.value=!1,U(e),Z())}function _e(e){K(e),f.value=!0}function ve(){f.value=!0}function ye(e){D.value?.$el.contains(e.relatedTarget)||(f.value=!1,U(e),Z())}function be(){D.value?.focus(),Z()}function xe(e){E.value&&(D.value?.$el.contains(ee(e))||Z())}function Se(t){if(!Array.isArray(t))return[];if(M.value)return Array.from(t);{let{remote:n}=e,{value:r}=w;if(n){let{value:e}=j;return t.filter(t=>r.has(t)||e.has(t))}return t.filter(e=>r.has(e))}}function we(e){Te(e.rawNode)}function Te(t){if(V.value)return;let{tag:n,remote:r,clearFilterAfterSelect:i,valueField:a}=e;if(n&&!r){let{value:e}=v,t=e[0]||null;if(t){let e=_.value;e.length?e.push(t):_.value=[t],v.value=A}}if(r&&j.value.set(t[a],t),e.multiple){let e=Se(d.value),o=e.findIndex(e=>e===t[a]);if(~o){if(e.splice(o,1),n&&!r){let e=Ee(t[a]);~e&&(_.value.splice(e,1),i&&(m.value=``))}}else e.push(t[a]),i&&(m.value=``);H(e,P(e))}else{if(n&&!r){let e=Ee(t[a]);~e?_.value=[_.value[e]]:_.value=A}Me(),Z(),H(t[a],t)}}function Ee(t){return _.value.findIndex(n=>n[e.valueField]===t)}function De(t){E.value||X();let{value:n}=t.target;m.value=n;let{tag:r,remote:i}=e;if(q(n),r&&!i){if(!n){v.value=A;return}let{onCreate:t}=e,r=t?t(n):{[e.labelField]:n,[e.valueField]:n},{valueField:i,labelField:a}=e;g.value.some(e=>e[i]===r[i]||e[a]===r[a])||_.value.some(e=>e[i]===r[i]||e[a]===r[a])?v.value=A:v.value=[r]}}function Oe(t){t.stopPropagation();let{multiple:n,tag:r,remote:i,clearCreatedOptionsOnClear:a}=e;!n&&e.filterable&&Z(),r&&!i&&a&&(_.value=A),W(),n?H([],[]):H(null,null)}function ke(e){J(e,`action`)||J(e,`empty`)||J(e,`header`)||e.preventDefault()}function Ae(e){ie(e)}function je(t){if(!e.keyboard)t.preventDefault();else switch(t.key){case` `:if(e.filterable)break;t.preventDefault();case`Enter`:if(!D.value?.isComposing){if(E.value){let t=k.value?.getPendingTmNode();t?we(t):e.filterable||(Z(),Me())}else if(X(),e.tag&&Q.value){let t=v.value[0];if(t){let n=t[e.valueField],{value:r}=d;e.multiple&&Array.isArray(r)&&r.includes(n)||Te(t)}}}t.preventDefault();break;case`ArrowUp`:if(t.preventDefault(),e.loading)return;E.value&&k.value?.prev();break;case`ArrowDown`:if(t.preventDefault(),e.loading)return;E.value?k.value?.next():X();break;case`Escape`:E.value&&(y(t),Z()),D.value?.focus()}}function Me(){D.value?.focus()}function Ne(){D.value?.focusInput()}function Pe(){E.value&&O.value?.syncPosition()}ae(),G(N(e,`options`),ae);let Fe={focus:()=>{D.value?.focus()},focusInput:()=>{D.value?.focusInput()},blur:()=>{D.value?.blur()},blurInput:()=>{D.value?.blurInput()}},Be=R(()=>{let{self:{menuBoxShadow:e}}=s.value;return{"--n-menu-box-shadow":e}}),Ve=a?c(`select`,void 0,Be,e):void 0;return{...Fe,mergedStatus:re,mergedClsPrefix:t,mergedBordered:n,namespace:i,treeMate:C,isMounted:fe(),triggerRef:D,menuRef:k,pattern:m,uncontrolledShow:T,mergedShow:E,adjustedTo:ce(e),uncontrolledValue:l,mergedValue:d,followerRef:O,localizedPlaceholder:ne,selectedOption:L,selectedOptions:F,mergedSize:B,mergedDisabled:V,focused:f,activeWithoutMenuOpen:Q,inlineThemeDisabled:a,onTriggerInputFocus:ue,onTriggerInputBlur:de,handleTriggerOrMenuResize:Pe,handleMenuFocus:ve,handleMenuBlur:ye,handleMenuTabOut:be,handleTriggerClick:$,handleToggle:we,handleDeleteOption:Te,handlePatternInput:De,handleClear:Oe,handleTriggerBlur:ge,handleTriggerFocus:_e,handleKeydown:je,handleMenuAfterLeave:le,handleMenuClickOutside:xe,handleMenuScroll:Ae,handleMenuKeydown:je,handleMenuMousedown:ke,mergedTheme:s,cssVars:a?void 0:Be,themeClass:Ve?.themeClass,onRender:Ve?.onRender}},render(){return P(),B(`div`,{class:a(`${this.mergedClsPrefix}-select`)},[L(ue,null,{_:1,default:e(()=>[(P(),W(le,null,{_:1,default:e(()=>(P(),W(qe,{ref:`triggerRef`,inlineThemeDisabled:this.inlineThemeDisabled,status:this.mergedStatus,inputProps:this.inputProps,clsPrefix:this.mergedClsPrefix,showArrow:this.showArrow,maxTagCount:this.maxTagCount,ellipsisTagPopoverProps:this.ellipsisTagPopoverProps,bordered:this.mergedBordered,active:this.activeWithoutMenuOpen||this.mergedShow,pattern:this.pattern,placeholder:this.localizedPlaceholder,selectedOption:this.selectedOption,selectedOptions:this.selectedOptions,multiple:this.multiple,renderTag:this.renderTag,renderLabel:this.renderLabel,filterable:this.filterable,clearable:this.clearable,disabled:this.mergedDisabled,size:this.mergedSize,theme:this.mergedTheme.peers.InternalSelection,labelField:this.labelField,valueField:this.valueField,themeOverrides:this.mergedTheme.peerOverrides.InternalSelection,loading:this.loading,focused:this.focused,onClick:this.handleTriggerClick,onDeleteOption:this.handleDeleteOption,onPatternInput:this.handlePatternInput,onClear:this.handleClear,onBlur:this.handleTriggerBlur,onFocus:this.handleTriggerFocus,onKeydown:this.handleKeydown,onPatternBlur:this.onTriggerInputBlur,onPatternFocus:this.onTriggerInputFocus,onResize:this.handleTriggerOrMenuResize,ignoreComposition:this.ignoreComposition},{_:1,arrow:e(()=>[this.$slots.arrow?.()])},8,`inlineThemeDisabled.status.inputProps.clsPrefix.showArrow.maxTagCount.ellipsisTagPopoverProps.bordered.active.pattern.placeholder.selectedOption.selectedOptions.multiple.renderTag.renderLabel.filterable.clearable.disabled.size.theme.labelField.valueField.themeOverrides.loading.focused.onClick.onDeleteOption.onPatternInput.onClear.onBlur.onFocus.onKeydown.onPatternBlur.onPatternFocus.onResize.ignoreComposition`.split(`.`))))})),(P(),W(de,{ref:`followerRef`,show:this.mergedShow,to:this.adjustedTo,teleportDisabled:this.adjustedTo===ce.tdkey,containerClass:this.namespace,width:this.consistentMenuWidth?`target`:void 0,minWidth:`target`,placement:this.placement},{_:1,default:e(()=>(P(),W(q,{name:`fade-in-scale-up-transition`,appear:this.isMounted,onAfterLeave:this.handleMenuAfterLeave},{_:1,default:e(()=>this.mergedShow||this.displayDirective===`show`?(this.onRender?.(),U((P(),W(Ne,H(this.menuProps,{ref:`menuRef`,onResize:this.handleTriggerOrMenuResize,inlineThemeDisabled:this.inlineThemeDisabled,virtualScroll:this.consistentMenuWidth&&this.virtualScroll,class:[`${this.mergedClsPrefix}-select-menu`,this.themeClass,this.menuProps?.class],clsPrefix:this.mergedClsPrefix,focusable:!0,labelField:this.labelField,valueField:this.valueField,autoPending:!0,nodeProps:this.nodeProps,theme:this.mergedTheme.peers.InternalSelectMenu,themeOverrides:this.mergedTheme.peerOverrides.InternalSelectMenu,treeMate:this.treeMate,multiple:this.multiple,size:this.menuSize,renderOption:this.renderOption,renderLabel:this.renderLabel,value:this.mergedValue,style:[this.menuProps?.style,this.cssVars],onToggle:this.handleToggle,onScroll:this.handleMenuScroll,onFocus:this.handleMenuFocus,onBlur:this.handleMenuBlur,onKeydown:this.handleMenuKeydown,onTabOut:this.handleMenuTabOut,onMousedown:this.handleMenuMousedown,show:this.mergedShow,showCheckmark:this.showCheckmark,resetMenuOnOptionsChange:this.resetMenuOnOptionsChange,scrollbarProps:this.scrollbarProps}),{_:1,empty:e(()=>[this.$slots.empty?.()]),header:e(()=>[this.$slots.header?.()]),action:e(()=>[this.$slots.action?.()])},16,`onResize.inlineThemeDisabled.virtualScroll.class.clsPrefix.labelField.valueField.nodeProps.theme.themeOverrides.treeMate.multiple.size.renderOption.renderLabel.value.style.onToggle.onScroll.onFocus.onBlur.onKeydown.onTabOut.onMousedown.show.showCheckmark.resetMenuOnOptionsChange.scrollbarProps`.split(`.`))),this.displayDirective===`show`?[[K,this.mergedShow],[b,this.handleMenuClickOutside,void 0,{capture:!0}]]:[[b,this.handleMenuClickOutside,void 0,{capture:!0}]])):null)},8,[`appear`,`onAfterLeave`])))},8,[`show`,`to`,`teleportDisabled`,`containerClass`,`width`,`placement`]))])})],2)}});export{De as a,Ne as i,qe as n,Te as o,Le as r,we as s,Xe as t};