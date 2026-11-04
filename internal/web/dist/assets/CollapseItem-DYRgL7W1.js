import{D as e,_ as t,at as n,c as r,ct as i,g as a,i as o,it as s,k as c,l,lt as u,o as d,st as f,v as p,w as m,y as h}from"./resolve-slot-D0ESJhJD.js";import{n as g,t as _}from"./use-rtl-B41EZXdW.js";import{D as v}from"./light-D4diJaDS.js";import{Et as y}from"./user-Cf_zUnVY.js";import{n as b}from"./replaceable-CG1Qs4lR.js";import{H as x,O as S,S as C,St as w,V as T,Wt as E,_t as D,d as O,f as k,h as A,ot as j,p as M}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{a as N}from"./runtime-dom.esm-bundler-B3Pl3ttw.js";import{t as P}from"./happens-in-D4nrdV77.js";import{t as F}from"./misc-DDs3MKLt.js";import{p as I}from"./vue-i18n-D4LRfH1J.js";import{o as L}from"./icon-switch.cssr-o4i5UUsg.js";import{t as R}from"./use-merged-state-KQOIh2IK.js";import{d as z}from"./Button-DUNxlS0h.js";import{t as B}from"./ChevronLeft-Bb8WFTYP.js";import{t as V}from"./ChevronRight-CSGCWNSb.js";var H=n(`collapse`,`width: 100%;`,[n(`collapse-item`,`
 font-size: var(--n-font-size);
 color: var(--n-text-color);
 transition:
 color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 margin: var(--n-item-margin);
 `,[i(`disabled`,[f(`header`,`cursor: not-allowed;`,[f(`header-main`,`
 color: var(--n-title-text-color-disabled);
 `),n(`collapse-item-arrow`,`
 color: var(--n-arrow-color-disabled);
 `)])]),n(`collapse-item`,`margin-left: 32px;`),s(`&:first-child`,`margin-top: 0;`),s(`&:first-child >`,[f(`header`,`padding-top: 0;`)]),i(`left-arrow-placement`,[f(`header`,[n(`collapse-item-arrow`,`margin-right: 4px;`)])]),i(`right-arrow-placement`,[f(`header`,[n(`collapse-item-arrow`,`margin-left: 4px;`)])]),f(`content-wrapper`,[f(`content-inner`,`padding-top: 16px;`),I({duration:`0.15s`})]),i(`active`,[f(`header`,[i(`active`,[n(`collapse-item-arrow`,`transform: rotate(90deg);`)])])]),s(`&:not(:first-child)`,`border-top: 1px solid var(--n-divider-color);`),u(`disabled`,[i(`trigger-area-main`,[f(`header`,[f(`header-main`,`cursor: pointer;`),n(`collapse-item-arrow`,`cursor: default;`)])]),i(`trigger-area-arrow`,[f(`header`,[n(`collapse-item-arrow`,`cursor: pointer;`)])]),i(`trigger-area-extra`,[f(`header`,[f(`header-extra`,`cursor: pointer;`)])])]),f(`header`,`
 font-size: var(--n-title-font-size);
 display: flex;
 flex-wrap: nowrap;
 align-items: center;
 transition: color .3s var(--n-bezier);
 position: relative;
 padding: var(--n-title-padding);
 color: var(--n-title-text-color);
 `,[f(`header-main`,`
 display: flex;
 flex-wrap: nowrap;
 align-items: center;
 font-weight: var(--n-title-font-weight);
 transition: color .3s var(--n-bezier);
 flex: 1;
 color: var(--n-title-text-color);
 `),f(`header-extra`,`
 display: flex;
 align-items: center;
 transition: color .3s var(--n-bezier);
 color: var(--n-text-color);
 `),n(`collapse-item-arrow`,`
 display: flex;
 transition:
 transform .15s var(--n-bezier),
 color .3s var(--n-bezier);
 font-size: 18px;
 color: var(--n-arrow-color);
 `)])])]),U={...r.props,defaultExpandedNames:{type:[Array,String],default:null},expandedNames:[Array,String],arrowPlacement:{type:String,default:`left`},accordion:Boolean,displayDirective:{type:String,default:`if`},triggerAreas:{type:Array,default:()=>[`main`,`extra`,`arrow`]},onItemHeaderClick:[Function,Array],"onUpdate:expandedNames":[Function,Array],onUpdateExpandedNames:[Function,Array],onExpandedNamesChange:{type:[Function,Array],validator:()=>!0,default:void 0}},W=e(`n-collapse`),G=C({name:`Collapse`,props:U,slots:Object,setup(e,{slots:t}){let{mergedClsPrefixRef:n,inlineThemeDisabled:i,mergedRtlRef:a}=m(e),o=D(e.defaultExpandedNames),s=O(()=>e.expandedNames),c=R(s,o),u=r(`Collapse`,`-collapse`,H,y,e,n);function d(t){let{"onUpdate:expandedNames":n,onUpdateExpandedNames:r,onExpandedNamesChange:i}=e;r&&g(r,t),n&&g(n,t),i&&g(i,t),o.value=t}function f(t){let{onItemHeaderClick:n}=e;n&&g(n,t)}function p(t,n,r){let{accordion:i}=e,{value:a}=c;if(i)t?(d([n]),f({name:n,expanded:!0,event:r})):(d([]),f({name:n,expanded:!1,event:r}));else if(!Array.isArray(a))d([n]),f({name:n,expanded:!0,event:r});else{let e=a.slice(),t=e.findIndex(e=>n===e);~t?(e.splice(t,1),d(e),f({name:n,expanded:!1,event:r})):(e.push(n),d(e),f({name:n,expanded:!0,event:r}))}}x(W,{props:e,mergedClsPrefixRef:n,expandedNamesRef:c,slots:t,toggleItem:p});let h=_(`Collapse`,a,n),v=O(()=>{let{common:{cubicBezierEaseInOut:e},self:{titleFontWeight:t,dividerColor:n,titlePadding:r,titleTextColor:i,titleTextColorDisabled:a,textColor:o,arrowColor:s,fontSize:c,titleFontSize:l,arrowColorDisabled:d,itemMargin:f}}=u.value;return{"--n-font-size":c,"--n-bezier":e,"--n-text-color":o,"--n-divider-color":n,"--n-title-padding":r,"--n-title-font-size":l,"--n-title-text-color":i,"--n-title-text-color-disabled":a,"--n-title-font-weight":t,"--n-arrow-color":s,"--n-arrow-color-disabled":d,"--n-item-margin":f}}),b=i?l(`collapse`,void 0,v,e):void 0;return{rtlEnabled:h,mergedTheme:u,mergedClsPrefix:n,cssVars:i?void 0:v,themeClass:b?.themeClass,onRender:b?.onRender}},render(){return this.onRender?.(),T(),A(`div`,{class:a([`${this.mergedClsPrefix}-collapse`,this.rtlEnabled&&`${this.mergedClsPrefix}-collapse--rtl`,this.themeClass]),style:E(this.cssVars)},[h(()=>this.$slots.default?.())],6)}}),K=C({name:`CollapseItemContent`,props:{displayDirective:{type:String,required:!0},show:Boolean,clsPrefix:{type:String,required:!0}},setup(e){return{onceTrue:v(w(e,`show`))}},render(){return T(),M(z,null,{_:1,default:t(()=>{let{show:e,displayDirective:t,onceTrue:n,clsPrefix:r}=this,i=t===`show`&&n,o=(T(),A(`div`,{class:a(`${r}-collapse-item__content-wrapper`)},[k(`div`,{class:a(`${r}-collapse-item__content-inner`)},[h(()=>this.$slots.default?.())],2)],2));return i?j(o,[[N,e]]):e?o:null})})}}),q=[`onClick`],J=[`onClick`],Y=C({name:`CollapseItem`,props:{title:String,name:[String,Number],disabled:Boolean,displayDirective:String},setup(e){let{mergedRtlRef:t}=m(e),n=F(),r=L(()=>e.name??n),i=S(W);i||c(`collapse-item`,"`n-collapse-item` must be placed inside `n-collapse`.");let{expandedNamesRef:a,props:o,mergedClsPrefixRef:s,slots:l}=i,u=O(()=>{let{value:e}=a;if(Array.isArray(e)){let{value:t}=r;return!~e.findIndex(e=>e===t)}if(e){let{value:t}=r;return t!==e}return!0});return{rtlEnabled:_(`Collapse`,t,s),collapseSlots:l,randomName:n,mergedClsPrefix:s,collapsed:u,triggerAreas:w(o,`triggerAreas`),mergedDisplayDirective:O(()=>{let{displayDirective:t}=e;return t||o.displayDirective}),arrowPlacement:O(()=>o.arrowPlacement),handleClick(t){let n=`main`;P(t,`arrow`)&&(n=`arrow`),P(t,`extra`)&&(n=`extra`),o.triggerAreas.includes(n)&&i&&!e.disabled&&i.toggleItem(u.value,r.value,t)}}},render(){let{collapseSlots:e,$slots:t,arrowPlacement:n,collapsed:r,mergedDisplayDirective:i,mergedClsPrefix:s,disabled:c,triggerAreas:l}=this,u=o(t.header,{collapsed:r},()=>[this.title]),f=t[`header-extra`]||e[`header-extra`],m=t.arrow||e.arrow;return T(),A(`div`,{class:a([`${s}-collapse-item`,`${s}-collapse-item--${n}-arrow-placement`,c&&`${s}-collapse-item--disabled`,!r&&`${s}-collapse-item--active`,l.map(e=>`${s}-collapse-item--trigger-area-${e}`)])},[k(`div`,{class:a([`${s}-collapse-item__header`,!r&&`${s}-collapse-item__header--active`])},[k(`div`,{class:a(`${s}-collapse-item__header-main`),onClick:this.handleClick},[h(()=>n===`right`&&u),(T(),A(`div`,{class:a(`${s}-collapse-item-arrow`),key:+!this.rtlEnabled,"data-arrow":!0},[h(()=>o(m,{collapsed:r},()=>[(T(),M(b,{clsPrefix:s},{default:()=>this.rtlEnabled?(T(),M(B,{key:1})):(T(),M(V,{key:2}))},1032,[`clsPrefix`]))]))],2)),h(()=>n===`left`&&u)],10,J),h(()=>d(f,{collapsed:r},e=>(T(),A(`div`,{class:a(`${s}-collapse-item__header-extra`),onClick:this.handleClick,"data-extra":!0},[h(()=>e)],10,q))))],2),(T(),M(K,{clsPrefix:s,displayDirective:i,show:!r},p(t),1032,[`clsPrefix`,`displayDirective`,`show`]))],2)}});export{G as n,Y as t};