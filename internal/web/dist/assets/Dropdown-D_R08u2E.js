import{A as e,D as t,_ as n,at as r,c as i,ct as a,g as o,it as s,l as c,lt as l,st as u,ut as d,w as f,y as p}from"./resolve-slot-D0ESJhJD.js";import{n as m}from"./use-rtl-B41EZXdW.js";import{M as h,O as g,S as _,f as v,k as y,p as b}from"./light-D4diJaDS.js";import{_t as x,xt as S}from"./user-BFrG88cX.js";import{n as C,r as w,t as T}from"./Popover-DfEILjfp.js";import{n as E}from"./Scrollbar-D9NqNphl.js";import{E as D,H as O,O as k,S as A,St as j,V as M,_t as N,d as P,f as F,h as I,i as L,j as R,p as z,rt as B}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{t as ee}from"./runtime-dom.esm-bundler-B3Pl3ttw.js";import{t as V}from"./happens-in-D4nrdV77.js";import{n as te,r as H,t as U}from"./Follower-B00Pedqi.js";import{o as W}from"./icon-switch.cssr-o4i5UUsg.js";import{t as G}from"./use-merged-state-KQOIh2IK.js";import{t as ne}from"./use-keyboard-CfUVlb0z.js";import{t as re}from"./format-length-BwSo_EUP.js";import{t as ie}from"./create-DH59f11n.js";import{t as ae}from"./ChevronRight-CSGCWNSb.js";import{t as oe}from"./create-ref-setter-AIudcg5Z.js";var se=r(`icon`,`
 height: 1em;
 width: 1em;
 line-height: 1em;
 text-align: center;
 display: inline-block;
 position: relative;
 fill: currentColor;
`,[a(`color-transition`,{transition:`color .3s var(--n-bezier)`}),a(`depth`,{color:`var(--n-color)`},[s(`svg`,{opacity:`var(--n-opacity)`,transition:`opacity .3s var(--n-bezier)`})]),s(`svg`,{height:`1em`,width:`1em`})]),ce={...i.props,depth:[String,Number],size:[Number,String],color:String,component:[Object,Function]},le=A({_n_icon__:!0,name:`Icon`,inheritAttrs:!1,props:ce,setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:n}=f(e),r=i(`Icon`,`-icon`,se,x,e,t),a=P(()=>{let{depth:t}=e,{common:{cubicBezierEaseInOut:n},self:i}=r.value;if(t!==void 0){let{color:e,[`opacity${t}Depth`]:r}=i;return{"--n-bezier":n,"--n-color":e,"--n-opacity":r}}return{"--n-bezier":n,"--n-color":``,"--n-opacity":``}}),o=n?c(`icon`,P(()=>`${e.depth||`d`}`),a,e):void 0;return{mergedClsPrefix:t,mergedStyle:P(()=>{let{size:t,color:n}=e;return{fontSize:re(t),color:n}}),cssVars:n?void 0:a,themeClass:o?.themeClass,onRender:o?.onRender}},render(){let{$parent:t,depth:n,mergedClsPrefix:r,component:i,onRender:a,themeClass:o}=this;return t?.$options?._n_icon__&&e(`icon`,"don't wrap `n-icon` inside `n-icon`"),a?.(),D(`i`,R(this.$attrs,{role:`img`,class:[`${r}-icon`,o,{[`${r}-icon--depth`]:n,[`${r}-icon--color-transition`]:n!==void 0}],style:[this.cssVars,this.mergedStyle]}),i?D(i):this.$slots.default?.())}}),K=t(`n-dropdown-menu`),q=t(`n-dropdown`),J=t(`n-dropdown-option`),Y=A({name:`DropdownDivider`,props:{clsPrefix:{type:String,required:!0}},render(){return M(),I(`div`,{class:o(`${this.clsPrefix}-dropdown-divider`)},null,2)}});function X(e,t){return e.type===`submenu`||e.type===void 0&&e[t]!==void 0}function ue(e){return e.type===`group`}function Z(e){return e.type===`divider`}function de(e){return e.type===`render`}function fe(e,t,n){if(!t)return e;let r=N(e.value),i=null;return B(e,e=>{i!==null&&window.clearTimeout(i),e===!0?n&&!n.value?r.value=!0:i=window.setTimeout(()=>{r.value=!0},t):r.value=!1}),r}var Q=A({name:`DropdownOption`,props:{clsPrefix:{type:String,required:!0},tmNode:{type:Object,required:!0},parentKey:{type:[String,Number],default:null},placement:{type:String,default:`right-start`},props:Object,scrollable:Boolean},setup(e){let t=k(q),{hoverKeyRef:n,keyboardKeyRef:r,lastToggledSubmenuKeyRef:i,pendingKeyPathRef:a,activeKeyPathRef:o,animatedRef:s,mergedShowRef:c,renderLabelRef:l,renderIconRef:u,labelFieldRef:d,childrenFieldRef:f,renderOptionRef:p,nodePropsRef:m,menuPropsRef:h}=t,_=k(J,null),v=k(K),y=k(g),b=P(()=>e.tmNode.rawNode),x=P(()=>{let{value:t}=f;return X(e.tmNode.rawNode,t)}),S=P(()=>{let{disabled:t}=e.tmNode;return t}),C=fe(P(()=>{if(!x.value)return!1;let{key:t,disabled:o}=e.tmNode;if(o)return!1;let{value:s}=n,{value:c}=r,{value:l}=i,{value:u}=a;return s===null?c===null?l!==null&&u.includes(t):u.includes(t)&&u[u.length-1]!==t:u.includes(t)}),300,P(()=>r.value===null&&!s.value)),w=P(()=>!!_?.enteringSubmenuRef.value),T=N(!1);O(J,{enteringSubmenuRef:T});function E(){T.value=!0}function D(){T.value=!1}function A(){let{parentKey:t,tmNode:a}=e;a.disabled||c.value&&(i.value=t,r.value=null,n.value=a.key)}function j(){let{tmNode:t}=e;t.disabled||c.value&&n.value!==t.key&&A()}function M(t){if(e.tmNode.disabled||!c.value)return;let{relatedTarget:r}=t;r&&!V({target:r},`dropdownOption`)&&!V({target:r},`scrollbarRail`)&&(n.value=null)}function F(){let{value:n}=x,{tmNode:r}=e;c.value&&!n&&!r.disabled&&(t.doSelect(r.key,r.rawNode),t.doUpdateShow(!1))}return{labelField:d,renderLabel:l,renderIcon:u,siblingHasIcon:v.showIconRef,siblingHasSubmenu:v.hasSubmenuRef,menuProps:h,popoverBody:y,animated:s,mergedShowSubmenu:P(()=>C.value&&!w.value),rawNode:b,hasSubmenu:x,pending:W(()=>{let{value:t}=a,{key:n}=e.tmNode;return t.includes(n)}),childActive:W(()=>{let{value:t}=o,{key:n}=e.tmNode,r=t.findIndex(e=>n===e);return r!==-1&&r<t.length-1}),active:W(()=>{let{value:t}=o,{key:n}=e.tmNode,r=t.findIndex(e=>n===e);return r!==-1&&r===t.length-1}),mergedDisabled:S,renderOption:p,nodeProps:m,handleClick:F,handleMouseMove:j,handleMouseEnter:A,handleMouseLeave:M,handleSubmenuBeforeEnter:E,handleSubmenuAfterEnter:D}},render(){let{animated:e,rawNode:t,mergedShowSubmenu:r,clsPrefix:i,siblingHasIcon:a,siblingHasSubmenu:s,renderLabel:c,renderIcon:l,renderOption:u,nodeProps:d,props:f,scrollable:m}=this,h=null;if(r){let e=this.menuProps?.(t,t.children);h=(t=>(M(),z($,R({key:1},e,{clsPrefix:i,scrollable:this.scrollable,tmNodes:this.tmNode.children,parentKey:this.tmNode.key}),null,16,[`clsPrefix`,`scrollable`,`tmNodes`,`parentKey`])))(h)}let g={class:[`${i}-dropdown-option-body`,this.pending&&`${i}-dropdown-option-body--pending`,this.active&&`${i}-dropdown-option-body--active`,this.childActive&&`${i}-dropdown-option-body--child-active`,this.mergedDisabled&&`${i}-dropdown-option-body--disabled`],onMousemove:this.handleMouseMove,onMouseenter:this.handleMouseEnter,onMouseleave:this.handleMouseLeave,onClick:this.handleClick},_=d?.(t),v=(M(),I(`div`,R({class:[`${i}-dropdown-option`,_?.class],"data-dropdown-option":!0},_),[p(()=>D(`div`,R(g,f),[(M(),I(`div`,{class:o([`${i}-dropdown-option-body__prefix`,a&&`${i}-dropdown-option-body__prefix--show-icon`])},[p(()=>[l?l(t):b(t.icon)])],2)),(M(),I(`div`,{"data-dropdown-option":!0,class:o(`${i}-dropdown-option-body__label`)},[c?(M(),I(L,{key:0},[p(()=>c(t))],64)):(M(),I(L,{key:1},[p(()=>b(t[this.labelField]??t.title))],64))],2)),(M(),I(`div`,{"data-dropdown-option":!0,class:o([`${i}-dropdown-option-body__suffix`,s&&`${i}-dropdown-option-body__suffix--has-submenu`])},[this.hasSubmenu?(M(),z(le,{key:0},{_:1,default:n(()=>(M(),z(ae)))})):p(()=>null)],2))])),this.hasSubmenu?(M(),z(H,{key:0},{default:()=>[(M(),z(te,null,{default:()=>(M(),I(`div`,{class:o(`${i}-dropdown-offset-container`)},[(M(),z(U,{show:this.mergedShowSubmenu,placement:this.placement,to:m&&this.popoverBody||void 0,teleportDisabled:!m},{default:()=>(M(),I(`div`,{class:o(`${i}-dropdown-menu-wrapper`)},[e?(M(),z(ee,{key:0,onBeforeEnter:this.handleSubmenuBeforeEnter,onAfterEnter:this.handleSubmenuAfterEnter,name:`fade-in-scale-up-transition`,appear:!0},{default:()=>h},1032,[`onBeforeEnter`,`onAfterEnter`])):(M(),I(L,{key:1},[p(()=>h)],64))],2))},1032,[`show`,`placement`,`to`,`teleportDisabled`]))],2))},1024))]},1024)):p(()=>null)],16));return u?u({node:v,option:t}):v}}),pe=A({name:`DropdownGroupHeader`,props:{clsPrefix:{type:String,required:!0},tmNode:{type:Object,required:!0}},setup(){let{showIconRef:e,hasSubmenuRef:t}=k(K),{renderLabelRef:n,labelFieldRef:r,nodePropsRef:i,renderOptionRef:a}=k(q);return{labelField:r,showIcon:e,hasSubmenu:t,renderLabel:n,nodeProps:i,renderOption:a}},render(){let{clsPrefix:e,hasSubmenu:t,showIcon:n,nodeProps:r,renderLabel:i,renderOption:a}=this,{rawNode:s}=this.tmNode,c=(M(),I(`div`,R({class:`${e}-dropdown-option`},r?.(s)),[F(`div`,{class:o(`${e}-dropdown-option-body ${e}-dropdown-option-body--group`)},[F(`div`,{"data-dropdown-option":!0,class:o([`${e}-dropdown-option-body__prefix`,n&&`${e}-dropdown-option-body__prefix--show-icon`])},[p(()=>b(s.icon))],2),F(`div`,{class:o(`${e}-dropdown-option-body__label`),"data-dropdown-option":!0},[i?(M(),I(L,{key:0},[p(()=>i(s))],64)):(M(),I(L,{key:1},[p(()=>b(s.title??s[this.labelField]))],64))],2),F(`div`,{class:o([`${e}-dropdown-option-body__suffix`,t&&`${e}-dropdown-option-body__suffix--has-submenu`]),"data-dropdown-option":!0},null,2)],2)],16));return a?a({node:c,option:s}):c}}),me=A({name:`NDropdownGroup`,props:{clsPrefix:{type:String,required:!0},tmNode:{type:Object,required:!0},parentKey:{type:[String,Number],default:null}},render(){let{tmNode:t,parentKey:n,clsPrefix:r}=this,{children:i}=t;return M(),I(L,null,[(M(),z(pe,{clsPrefix:r,tmNode:t,key:t.key},null,8,[`clsPrefix`,`tmNode`])),p(()=>i?.map(t=>{let{rawNode:i}=t;return i.show===!1?null:Z(i)?D(Y,{clsPrefix:r,key:t.key}):t.isGroup?(e(`dropdown`,"`group` node is not allowed to be put in `group` node."),null):(M(),z(Q,{clsPrefix:r,tmNode:t,parentKey:n,key:t.key},null,8,[`clsPrefix`,`tmNode`,`parentKey`]))}))],64)}}),he=A({name:`DropdownRenderOption`,props:{tmNode:{type:Object,required:!0}},render(){let{rawNode:{render:e,props:t}}=this.tmNode;return D(`div`,t,[e?.()])}}),$=A({name:`DropdownMenu`,props:{scrollable:Boolean,showArrow:Boolean,arrowStyle:[String,Object],clsPrefix:{type:String,required:!0},tmNodes:{type:Array,default:()=>[]},parentKey:{type:[String,Number],default:null}},setup(e){let{renderIconRef:t,childrenFieldRef:n}=k(q);O(K,{showIconRef:P(()=>{let n=t.value;return e.tmNodes.some(e=>{if(e.isGroup)return e.children?.some(({rawNode:e})=>n?n(e):e.icon);let{rawNode:t}=e;return n?n(t):t.icon})}),hasSubmenuRef:P(()=>{let{value:t}=n;return e.tmNodes.some(e=>{if(e.isGroup)return e.children?.some(({rawNode:e})=>X(e,t));let{rawNode:n}=e;return X(n,t)})})});let r=N(null);return O(y,null),O(h,null),O(g,r),{bodyRef:r}},render(){let{parentKey:e,clsPrefix:t,scrollable:n}=this,r=this.tmNodes.map(r=>{let{rawNode:i}=r;return i.show===!1?null:de(i)?(M(),z(he,{tmNode:r,key:r.key},null,8,[`tmNode`])):Z(i)?(M(),z(Y,{clsPrefix:t,key:r.key},null,8,[`clsPrefix`])):ue(i)?(M(),z(me,{clsPrefix:t,tmNode:r,parentKey:e,key:r.key},null,8,[`clsPrefix`,`tmNode`,`parentKey`])):(M(),z(Q,{clsPrefix:t,tmNode:r,parentKey:e,key:r.key,props:i.props,scrollable:n},null,8,[`clsPrefix`,`tmNode`,`parentKey`,`props`,`scrollable`]))});return M(),I(`div`,{class:o([`${t}-dropdown-menu`,n&&`${t}-dropdown-menu--scrollable`]),ref:`bodyRef`},[n?(M(),z(E,{key:0,contentClass:`${t}-dropdown-menu__content`},{default:()=>r},1032,[`contentClass`])):(M(),I(L,{key:1},[p(()=>r)],64)),this.showArrow?(M(),I(L,{key:2},[p(()=>w({clsPrefix:t,arrowStyle:this.arrowStyle,arrowClass:void 0,arrowWrapperClass:void 0,arrowWrapperStyle:void 0}))],64)):p(()=>null)],2)}}),ge=r(`dropdown-menu`,`
 transform-origin: var(--v-transform-origin);
 background-color: var(--n-color);
 border-radius: var(--n-border-radius);
 box-shadow: var(--n-box-shadow);
 position: relative;
 transition:
 background-color .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier);
`,[v(),r(`dropdown-option`,`
 position: relative;
 `,[s(`a`,`
 text-decoration: none;
 color: inherit;
 outline: none;
 `,[s(`&::before`,`
 content: "";
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 `)]),r(`dropdown-option-body`,`
 display: flex;
 cursor: pointer;
 position: relative;
 height: var(--n-option-height);
 line-height: var(--n-option-height);
 font-size: var(--n-font-size);
 color: var(--n-option-text-color);
 transition: color .3s var(--n-bezier);
 `,[s(`&::before`,`
 content: "";
 position: absolute;
 top: 0;
 bottom: 0;
 left: 4px;
 right: 4px;
 transition: background-color .3s var(--n-bezier);
 border-radius: var(--n-border-radius);
 `),l(`disabled`,[a(`pending`,`
 color: var(--n-option-text-color-hover);
 `,[u(`prefix, suffix`,`
 color: var(--n-option-text-color-hover);
 `),s(`&::before`,`background-color: var(--n-option-color-hover);`)]),a(`active`,`
 color: var(--n-option-text-color-active);
 `,[u(`prefix, suffix`,`
 color: var(--n-option-text-color-active);
 `),s(`&::before`,`background-color: var(--n-option-color-active);`)]),a(`child-active`,`
 color: var(--n-option-text-color-child-active);
 `,[u(`prefix, suffix`,`
 color: var(--n-option-text-color-child-active);
 `)])]),a(`disabled`,`
 cursor: not-allowed;
 opacity: var(--n-option-opacity-disabled);
 `),a(`group`,`
 font-size: calc(var(--n-font-size) - 1px);
 color: var(--n-group-header-text-color);
 `,[u(`prefix`,`
 width: calc(var(--n-option-prefix-width) / 2);
 `,[a(`show-icon`,`
 width: calc(var(--n-option-icon-prefix-width) / 2);
 `)])]),u(`prefix`,`
 width: var(--n-option-prefix-width);
 display: flex;
 justify-content: center;
 align-items: center;
 color: var(--n-prefix-color);
 transition: color .3s var(--n-bezier);
 z-index: 1;
 `,[a(`show-icon`,`
 width: var(--n-option-icon-prefix-width);
 `),r(`icon`,`
 font-size: var(--n-option-icon-size);
 `)]),u(`label`,`
 white-space: nowrap;
 flex: 1;
 z-index: 1;
 `),u(`suffix`,`
 box-sizing: border-box;
 flex-grow: 0;
 flex-shrink: 0;
 display: flex;
 justify-content: flex-end;
 align-items: center;
 min-width: var(--n-option-suffix-width);
 padding: 0 8px;
 transition: color .3s var(--n-bezier);
 color: var(--n-suffix-color);
 z-index: 1;
 `,[a(`has-submenu`,`
 width: var(--n-option-icon-suffix-width);
 `),r(`icon`,`
 font-size: var(--n-option-icon-size);
 `)]),r(`dropdown-menu`,`pointer-events: all;`)]),r(`dropdown-offset-container`,`
 pointer-events: none;
 position: absolute;
 left: 0;
 right: 0;
 top: -4px;
 bottom: -4px;
 `)]),r(`dropdown-divider`,`
 transition: background-color .3s var(--n-bezier);
 background-color: var(--n-divider-color);
 height: 1px;
 margin: 4px 0;
 `),r(`dropdown-menu-wrapper`,`
 transform-origin: var(--v-transform-origin);
 width: fit-content;
 `),s(`>`,[r(`scrollbar`,`
 height: inherit;
 max-height: inherit;
 `)]),l(`scrollable`,`
 padding: var(--n-padding);
 `),a(`scrollable`,[u(`content`,`
 padding: var(--n-padding);
 `)])]),_e={animated:{type:Boolean,default:!0},keyboard:{type:Boolean,default:!0},size:String,inverted:Boolean,placement:{type:String,default:`bottom`},onSelect:[Function,Array],options:{type:Array,default:()=>[]},menuProps:Function,showArrow:Boolean,renderLabel:Function,renderIcon:Function,renderOption:Function,nodeProps:Function,labelField:{type:String,default:`label`},keyField:{type:String,default:`key`},childrenField:{type:String,default:`children`},value:[String,Number]},ve=Object.keys(C),ye={...C,..._e,...i.props},be=A({name:`Dropdown`,inheritAttrs:!1,props:ye,setup(e){let t=N(!1),n=G(j(e,`show`),t),r=P(()=>{let{keyField:t,childrenField:n}=e;return ie(e.options,{getKey(e){return e[t]},getDisabled(e){return e.disabled===!0},getIgnored(e){return e.type===`divider`||e.type===`render`},getChildren(e){return e[n]}})}),a=P(()=>r.value.treeNodes),o=N(null),s=N(null),l=N(null),u=P(()=>o.value??s.value??l.value??null),p=P(()=>r.value.getPath(u.value).keyPath),h=P(()=>r.value.getPath(e.value).keyPath),g=W(()=>e.keyboard&&n.value);ne({keydown:{ArrowUp:{prevent:!0,handler:A},ArrowRight:{prevent:!0,handler:k},ArrowDown:{prevent:!0,handler:M},ArrowLeft:{prevent:!0,handler:D},Enter:{prevent:!0,handler:F},Escape:E}},g);let{mergedClsPrefixRef:_,inlineThemeDisabled:v,mergedComponentPropsRef:y}=f(e),b=P(()=>e.size||y?.value?.Dropdown?.size||`medium`),x=i(`Dropdown`,`-dropdown`,ge,S,e,_);O(q,{labelFieldRef:j(e,`labelField`),childrenFieldRef:j(e,`childrenField`),renderLabelRef:j(e,`renderLabel`),renderIconRef:j(e,`renderIcon`),hoverKeyRef:o,keyboardKeyRef:s,lastToggledSubmenuKeyRef:l,pendingKeyPathRef:p,activeKeyPathRef:h,animatedRef:j(e,`animated`),mergedShowRef:n,nodePropsRef:j(e,`nodeProps`),renderOptionRef:j(e,`renderOption`),menuPropsRef:j(e,`menuProps`),doSelect:C,doUpdateShow:w}),B(n,t=>{!e.animated&&!t&&T()});function C(t,n){let{onSelect:r}=e;r&&m(r,t,n)}function w(n){let{"onUpdate:show":r,onUpdateShow:i}=e;r&&m(r,n),i&&m(i,n),t.value=n}function T(){o.value=null,s.value=null,l.value=null}function E(){w(!1)}function D(){L(`left`)}function k(){L(`right`)}function A(){L(`up`)}function M(){L(`down`)}function F(){let e=I();e?.isLeaf&&n.value&&(C(e.key,e.rawNode),w(!1))}function I(){let{value:e}=r,{value:t}=u;return!e||t===null?null:e.getNode(t)??null}function L(e){let{value:t}=u,{value:{getFirstAvailableNode:n}}=r,i=null;if(t===null){let e=n();e!==null&&(i=e.key)}else{let t=I();if(t){let n;switch(e){case`down`:n=t.getNext();break;case`up`:n=t.getPrev();break;case`right`:n=t.getChild();break;case`left`:n=t.getParent()}n&&(i=n.key)}}i!==null&&(o.value=null,s.value=i)}let R=P(()=>{let{inverted:t}=e,n=b.value,{common:{cubicBezierEaseInOut:r},self:i}=x.value,{padding:a,dividerColor:o,borderRadius:s,optionOpacityDisabled:c,[d(`optionIconSuffixWidth`,n)]:l,[d(`optionSuffixWidth`,n)]:u,[d(`optionIconPrefixWidth`,n)]:f,[d(`optionPrefixWidth`,n)]:p,[d(`fontSize`,n)]:m,[d(`optionHeight`,n)]:h,[d(`optionIconSize`,n)]:g}=i,_={"--n-bezier":r,"--n-font-size":m,"--n-padding":a,"--n-border-radius":s,"--n-option-height":h,"--n-option-prefix-width":p,"--n-option-icon-prefix-width":f,"--n-option-suffix-width":u,"--n-option-icon-suffix-width":l,"--n-option-icon-size":g,"--n-divider-color":o,"--n-option-opacity-disabled":c};return t?(_[`--n-color`]=i.colorInverted,_[`--n-option-color-hover`]=i.optionColorHoverInverted,_[`--n-option-color-active`]=i.optionColorActiveInverted,_[`--n-option-text-color`]=i.optionTextColorInverted,_[`--n-option-text-color-hover`]=i.optionTextColorHoverInverted,_[`--n-option-text-color-active`]=i.optionTextColorActiveInverted,_[`--n-option-text-color-child-active`]=i.optionTextColorChildActiveInverted,_[`--n-prefix-color`]=i.prefixColorInverted,_[`--n-suffix-color`]=i.suffixColorInverted,_[`--n-group-header-text-color`]=i.groupHeaderTextColorInverted):(_[`--n-color`]=i.color,_[`--n-option-color-hover`]=i.optionColorHover,_[`--n-option-color-active`]=i.optionColorActive,_[`--n-option-text-color`]=i.optionTextColor,_[`--n-option-text-color-hover`]=i.optionTextColorHover,_[`--n-option-text-color-active`]=i.optionTextColorActive,_[`--n-option-text-color-child-active`]=i.optionTextColorChildActive,_[`--n-prefix-color`]=i.prefixColor,_[`--n-suffix-color`]=i.suffixColor,_[`--n-group-header-text-color`]=i.groupHeaderTextColor),_}),z=v?c(`dropdown`,P(()=>`${b.value[0]}${e.inverted?`i`:``}`),R,e):void 0;return{mergedClsPrefix:_,mergedTheme:x,mergedSize:b,tmNodes:a,mergedShow:n,handleAfterLeave:()=>{e.animated&&T()},doUpdateShow:w,cssVars:v?void 0:R,themeClass:z?.themeClass,onRender:z?.onRender}},render(){let e=(e,t,n,r,i)=>{let{mergedClsPrefix:a,menuProps:o}=this;this.onRender?.();let s=o?.(void 0,this.tmNodes.map(e=>e.rawNode))||{},c={ref:oe(t),class:[e,`${a}-dropdown`,`${a}-dropdown--${this.mergedSize}-size`,this.themeClass],clsPrefix:a,tmNodes:this.tmNodes,style:[...n,this.cssVars],showArrow:this.showArrow,arrowStyle:this.arrowStyle,scrollable:this.scrollable,onMouseenter:r,onMouseleave:i};return D($,R(this.$attrs,c,s))},{mergedTheme:t}=this,r={show:this.mergedShow,theme:t.peers.Popover,themeOverrides:t.peerOverrides.Popover,internalOnAfterLeave:this.handleAfterLeave,internalRenderBody:e,onUpdateShow:this.doUpdateShow,"onUpdate:show":void 0};return M(),z(T,_(this.$props,ve,r),{_:1,trigger:n(()=>this.$slots.default?.())},16)}});export{be as t};