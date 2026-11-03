import{D as e,a as t,at as n,c as r,ct as i,g as a,it as o,k as s,l as c,r as l,st as u,u as d,ut as f,w as p,y as m}from"./resolve-slot-D0ESJhJD.js";import{H as h,O as g,S as _,V as v,Wt as y,d as b,f as x,h as S}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{t as C}from"./format-length-BwSo_EUP.js";import{t as w}from"./is-browser-e-NGrvPW.js";import{t as T}from"./_common-CfZ8i3MN.js";var E=!1;function D(){if(w&&window.CSS&&!E&&(E=!0,`registerProperty`in window?.CSS))try{CSS.registerProperty({name:`--n-color-start`,syntax:`<color>`,inherits:!1,initialValue:`#0000`}),CSS.registerProperty({name:`--n-color-end`,syntax:`<color>`,inherits:!1,initialValue:`#0000`})}catch{}}function O(e){let{textColor3:t,infoColor:n,errorColor:r,successColor:i,warningColor:a,textColor1:o,textColor2:s,railColor:c,fontWeightStrong:l,fontSize:u}=e;return{...T,contentFontSize:u,titleFontWeight:l,circleBorder:`2px solid ${t}`,circleBorderInfo:`2px solid ${n}`,circleBorderError:`2px solid ${r}`,circleBorderSuccess:`2px solid ${i}`,circleBorderWarning:`2px solid ${a}`,iconColor:t,iconColorInfo:n,iconColorError:r,iconColorSuccess:i,iconColorWarning:a,titleTextColor:o,contentTextColor:s,metaTextColor:t,lineColor:c}}var k={name:`Timeline`,common:d,self:O},A=1.25,j=n(`timeline`,`
 position: relative;
 width: 100%;
 display: flex;
 flex-direction: column;
 line-height: ${A};
`,[i(`horizontal`,`
 flex-direction: row;
 `,[o(`>`,[n(`timeline-item`,`
 flex-shrink: 0;
 padding-right: 40px;
 `,[i(`dashed-line-type`,[o(`>`,[n(`timeline-item-timeline`,[u(`line`,`
 background-image: linear-gradient(90deg, var(--n-color-start), var(--n-color-start) 50%, transparent 50%, transparent 100%);
 background-size: 10px 1px;
 `)])])]),o(`>`,[n(`timeline-item-content`,`
 margin-top: calc(var(--n-icon-size) + 12px);
 `,[o(`>`,[u(`meta`,`
 margin-top: 6px;
 margin-bottom: unset;
 `)])]),n(`timeline-item-timeline`,`
 width: 100%;
 height: calc(var(--n-icon-size) + 12px);
 `,[u(`line`,`
 left: var(--n-icon-size);
 top: calc(var(--n-icon-size) / 2 - 1px);
 right: 0px;
 width: unset;
 height: 2px;
 `)])])])])]),i(`right-placement`,[n(`timeline-item`,[n(`timeline-item-content`,`
 text-align: right;
 margin-right: calc(var(--n-icon-size) + 12px);
 `),n(`timeline-item-timeline`,`
 width: var(--n-icon-size);
 right: 0;
 `)])]),i(`left-placement`,[n(`timeline-item`,[n(`timeline-item-content`,`
 margin-left: calc(var(--n-icon-size) + 12px);
 `),n(`timeline-item-timeline`,`
 left: 0;
 `)])]),n(`timeline-item`,`
 position: relative;
 `,[o(`&:last-child`,[n(`timeline-item-timeline`,[u(`line`,`
 display: none;
 `)]),n(`timeline-item-content`,[u(`meta`,`
 margin-bottom: 0;
 `)])]),n(`timeline-item-content`,[u(`title`,`
 margin: var(--n-title-margin);
 font-size: var(--n-title-font-size);
 transition: color .3s var(--n-bezier);
 font-weight: var(--n-title-font-weight);
 color: var(--n-title-text-color);
 `),u(`content`,`
 transition: color .3s var(--n-bezier);
 font-size: var(--n-content-font-size);
 color: var(--n-content-text-color);
 `),u(`meta`,`
 transition: color .3s var(--n-bezier);
 font-size: 12px;
 margin-top: 6px;
 margin-bottom: 20px;
 color: var(--n-meta-text-color);
 `)]),i(`dashed-line-type`,[n(`timeline-item-timeline`,[u(`line`,`
 --n-color-start: var(--n-line-color);
 transition: --n-color-start .3s var(--n-bezier);
 background-color: transparent;
 background-image: linear-gradient(180deg, var(--n-color-start), var(--n-color-start) 50%, transparent 50%, transparent 100%);
 background-size: 1px 10px;
 `)])]),n(`timeline-item-timeline`,`
 width: calc(var(--n-icon-size) + 12px);
 position: absolute;
 top: calc(var(--n-title-font-size) * ${A} / 2 - var(--n-icon-size) / 2);
 height: 100%;
 `,[u(`circle`,`
 border: var(--n-circle-border);
 transition:
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 width: var(--n-icon-size);
 height: var(--n-icon-size);
 border-radius: var(--n-icon-size);
 box-sizing: border-box;
 `),u(`icon`,`
 color: var(--n-icon-color);
 font-size: var(--n-icon-size);
 height: var(--n-icon-size);
 width: var(--n-icon-size);
 display: flex;
 align-items: center;
 justify-content: center;
 `),u(`line`,`
 transition: background-color .3s var(--n-bezier);
 position: absolute;
 top: var(--n-icon-size);
 left: calc(var(--n-icon-size) / 2 - 1px);
 bottom: 0px;
 width: 2px;
 background-color: var(--n-line-color);
 `)])])]),M={...r.props,horizontal:Boolean,itemPlacement:{type:String,default:`left`},size:{type:String,default:`medium`},iconSize:Number},N=e(`n-timeline`),P=_({name:`Timeline`,props:M,setup(e,{slots:t}){let{mergedClsPrefixRef:n}=p(e),i=r(`Timeline`,`-timeline`,j,k,e,n);return h(N,{props:e,mergedThemeRef:i,mergedClsPrefixRef:n}),()=>{let{value:r}=n;return v(),S(`div`,{class:a([`${r}-timeline`,e.horizontal&&`${r}-timeline--horizontal`,`${r}-timeline--${e.size}-size`,!e.horizontal&&`${r}-timeline--${e.itemPlacement}-placement`])},[m(()=>t.default?.())],2)}}}),F=_({name:`TimelineItem`,props:{time:[String,Number],title:String,content:String,color:String,lineType:{type:String,default:`default`},type:{type:String,default:`default`}},slots:Object,setup(e){let t=g(N);t||s(`timeline-item`,"`n-timeline-item` must be placed inside `n-timeline`."),D();let{inlineThemeDisabled:n}=p(),r=b(()=>{let{props:{size:n,iconSize:r},mergedThemeRef:i}=t,{type:a}=e,{self:{titleTextColor:o,contentTextColor:s,metaTextColor:c,lineColor:l,titleFontWeight:u,contentFontSize:d,[f(`iconSize`,n)]:p,[f(`titleMargin`,n)]:m,[f(`titleFontSize`,n)]:h,[f(`circleBorder`,a)]:g,[f(`iconColor`,a)]:_},common:{cubicBezierEaseInOut:v}}=i.value;return{"--n-bezier":v,"--n-circle-border":g,"--n-icon-color":_,"--n-content-font-size":d,"--n-content-text-color":s,"--n-line-color":l,"--n-meta-text-color":c,"--n-title-font-size":h,"--n-title-font-weight":u,"--n-title-margin":m,"--n-title-text-color":o,"--n-icon-size":C(r)||p}}),i=n?c(`timeline-item`,b(()=>{let{props:{size:n,iconSize:r}}=t,{type:i}=e;return`${n[0]}${r||`a`}${i[0]}`}),r,t.props):void 0;return{mergedClsPrefix:t.mergedClsPrefixRef,cssVars:n?void 0:r,themeClass:i?.themeClass,onRender:i?.onRender}},render(){let{mergedClsPrefix:e,color:n,onRender:r,$slots:i}=this;return r?.(),v(),S(`div`,{class:a([`${e}-timeline-item`,this.themeClass,`${e}-timeline-item--${this.type}-type`,`${e}-timeline-item--${this.lineType}-line-type`]),style:y(this.cssVars)},[x(`div`,{class:a(`${e}-timeline-item-timeline`)},[x(`div`,{class:a(`${e}-timeline-item-timeline__line`)},null,2),m(()=>t(i.icon,t=>t?(v(),S(`div`,{key:1,class:a(`${e}-timeline-item-timeline__icon`),style:y({color:n})},[m(()=>t)],6)):(v(),S(`div`,{key:2,class:a(`${e}-timeline-item-timeline__circle`),style:y({borderColor:n})},null,6))))],2),x(`div`,{class:a(`${e}-timeline-item-content`)},[m(()=>t(i.header,t=>t||this.title?(v(),S(`div`,{key:3,class:a(`${e}-timeline-item-content__title`)},[m(()=>t||this.title)],2)):null)),x(`div`,{class:a(`${e}-timeline-item-content__content`)},[m(()=>l(i.default,()=>[this.content]))],2),x(`div`,{class:a(`${e}-timeline-item-content__meta`)},[m(()=>l(i.footer,()=>[this.time]))],2)],2)],6)}});export{P as n,F as t};