import{a as e,at as t,c as n,ct as r,d as i,g as a,it as o,l as s,lt as c,n as l,st as u,u as d,ut as f,w as p,y as m}from"./resolve-slot-D0ESJhJD.js";import{n as h}from"./use-rtl-B41EZXdW.js";import{o as g,r as _}from"./replaceable-CG1Qs4lR.js";import{S as v,St as y,V as b,Wt as x,_t as S,d as C,f as w,h as T,j as E,p as D}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{a as O}from"./vue-i18n-D4LRfH1J.js";import{i as k,n as A,t as j}from"./icon-switch.cssr-o4i5UUsg.js";import{t as M}from"./use-merged-state-KQOIh2IK.js";import{l as N}from"./Button-DUNxlS0h.js";function P(e){let{primaryColor:t,opacityDisabled:n,borderRadius:r,textColor3:a}=e;return{...O,iconColor:a,textColor:`white`,loadingColor:t,opacityDisabled:n,railColor:`rgba(0, 0, 0, .14)`,railColorActive:t,buttonBoxShadow:`0 1px 4px 0 rgba(0, 0, 0, 0.3), inset 0 0 1px 0 rgba(0, 0, 0, 0.05)`,buttonColor:`#FFF`,railBorderRadiusSmall:r,railBorderRadiusMedium:r,railBorderRadiusLarge:r,buttonBorderRadiusSmall:r,buttonBorderRadiusMedium:r,buttonBorderRadiusLarge:r,boxShadowFocus:`0 0 0 2px ${i(t,{alpha:.2})}`}}var F={name:`Switch`,common:d,self:P},I=t(`switch`,`
 height: var(--n-height);
 min-width: var(--n-width);
 vertical-align: middle;
 user-select: none;
 -webkit-user-select: none;
 display: inline-flex;
 outline: none;
 justify-content: center;
 align-items: center;
`,[u(`children-placeholder`,`
 height: var(--n-rail-height);
 display: flex;
 flex-direction: column;
 overflow: hidden;
 pointer-events: none;
 visibility: hidden;
 `),u(`rail-placeholder`,`
 display: flex;
 flex-wrap: none;
 `),u(`button-placeholder`,`
 width: calc(1.75 * var(--n-rail-height));
 height: var(--n-rail-height);
 `),t(`base-loading`,`
 position: absolute;
 top: 50%;
 left: 50%;
 transform: translateX(-50%) translateY(-50%);
 font-size: calc(var(--n-button-width) - 4px);
 color: var(--n-loading-color);
 transition: color .3s var(--n-bezier);
 `,[j({left:`50%`,top:`50%`,originalTransform:`translateX(-50%) translateY(-50%)`})]),u(`checked, unchecked`,`
 transition: color .3s var(--n-bezier);
 color: var(--n-text-color);
 box-sizing: border-box;
 position: absolute;
 white-space: nowrap;
 top: 0;
 bottom: 0;
 display: flex;
 align-items: center;
 line-height: 1;
 `),u(`checked`,`
 right: 0;
 padding-right: calc(1.25 * var(--n-rail-height) - var(--n-offset));
 `),u(`unchecked`,`
 left: 0;
 justify-content: flex-end;
 padding-left: calc(1.25 * var(--n-rail-height) - var(--n-offset));
 `),o(`&:focus`,[u(`rail`,`
 box-shadow: var(--n-box-shadow-focus);
 `)]),r(`round`,[u(`rail`,`border-radius: calc(var(--n-rail-height) / 2);`,[u(`button`,`border-radius: calc(var(--n-button-height) / 2);`)])]),c(`disabled`,[c(`icon`,[r(`rubber-band`,[r(`pressed`,[u(`rail`,[u(`button`,`max-width: var(--n-button-width-pressed);`)])]),u(`rail`,[o(`&:active`,[u(`button`,`max-width: var(--n-button-width-pressed);`)])]),r(`active`,[r(`pressed`,[u(`rail`,[u(`button`,`left: calc(100% - var(--n-offset) - var(--n-button-width-pressed));`)])]),u(`rail`,[o(`&:active`,[u(`button`,`left: calc(100% - var(--n-offset) - var(--n-button-width-pressed));`)])])])])])]),r(`active`,[u(`rail`,[u(`button`,`left: calc(100% - var(--n-button-width) - var(--n-offset))`)])]),u(`rail`,`
 overflow: hidden;
 height: var(--n-rail-height);
 min-width: var(--n-rail-width);
 border-radius: var(--n-rail-border-radius);
 cursor: pointer;
 position: relative;
 transition:
 opacity .3s var(--n-bezier),
 background .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier);
 background-color: var(--n-rail-color);
 `,[u(`button-icon`,`
 color: var(--n-icon-color);
 transition: color .3s var(--n-bezier);
 font-size: calc(var(--n-button-height) - 4px);
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 display: flex;
 justify-content: center;
 align-items: center;
 line-height: 1;
 `,[j()]),u(`button`,`
 align-items: center; 
 top: var(--n-offset);
 left: var(--n-offset);
 height: var(--n-button-height);
 width: var(--n-button-width-pressed);
 max-width: var(--n-button-width);
 border-radius: var(--n-button-border-radius);
 background-color: var(--n-button-color);
 box-shadow: var(--n-button-box-shadow);
 box-sizing: border-box;
 cursor: inherit;
 content: "";
 position: absolute;
 transition:
 background-color .3s var(--n-bezier),
 left .3s var(--n-bezier),
 opacity .3s var(--n-bezier),
 max-width .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier);
 `)]),r(`active`,[u(`rail`,`background-color: var(--n-rail-color-active);`)]),r(`loading`,[u(`rail`,`
 cursor: wait;
 `)]),r(`disabled`,[u(`rail`,`
 cursor: not-allowed;
 opacity: .5;
 `)])]),L=[`aria-checked`,`tabindex`,`onClick`,`onFocus`,`onBlur`,`onKeyup`,`onKeydown`],R={...n.props,size:String,value:{type:[String,Number,Boolean],default:void 0},loading:Boolean,defaultValue:{type:[String,Number,Boolean],default:!1},disabled:{type:Boolean,default:void 0},round:{type:Boolean,default:!0},"onUpdate:value":[Function,Array],onUpdateValue:[Function,Array],checkedValue:{type:[String,Number,Boolean],default:!0},uncheckedValue:{type:[String,Number,Boolean],default:!1},railStyle:Function,rubberBand:{type:Boolean,default:!0},spinProps:Object,onChange:[Function,Array]},z,B=v({name:`Switch`,props:R,slots:Object,setup(e){z===void 0&&(z=typeof CSS<`u`?CSS.supports!==void 0&&CSS.supports(`width`,`max(1px)`):!0);let{mergedClsPrefixRef:t,inlineThemeDisabled:r,mergedComponentPropsRef:i}=p(e),a=n(`Switch`,`-switch`,I,F,e,t),o=k(e,{mergedSize(t){return e.size===void 0?t?t.mergedSize.value:i?.value?.Switch?.size||`medium`:e.size}}),{mergedSizeRef:c,mergedDisabledRef:l}=o,u=S(e.defaultValue),d=y(e,`value`),m=M(d,u),v=C(()=>m.value===e.checkedValue),b=S(!1),x=S(!1),w=C(()=>{let{railStyle:t}=e;if(t)return t({focused:x.value,checked:v.value})});function T(t){let{"onUpdate:value":n,onChange:r,onUpdateValue:i}=e,{nTriggerFormInput:a,nTriggerFormChange:s}=o;n&&h(n,t),i&&h(i,t),r&&h(r,t),u.value=t,a(),s()}function E(){let{nTriggerFormFocus:e}=o;e()}function D(){let{nTriggerFormBlur:e}=o;e()}function O(){e.loading||l.value||(m.value===e.checkedValue?T(e.uncheckedValue):T(e.checkedValue))}function A(){x.value=!0,E()}function j(){x.value=!1,D(),b.value=!1}function N(t){e.loading||l.value||t.key===` `&&(m.value===e.checkedValue?T(e.uncheckedValue):T(e.checkedValue),b.value=!1)}function P(t){e.loading||l.value||t.key===` `&&(t.preventDefault(),b.value=!0)}let L=C(()=>{let{value:e}=c,{self:{opacityDisabled:t,railColor:n,railColorActive:r,buttonBoxShadow:i,buttonColor:o,boxShadowFocus:s,loadingColor:l,textColor:u,iconColor:d,[f(`buttonHeight`,e)]:p,[f(`buttonWidth`,e)]:m,[f(`buttonWidthPressed`,e)]:h,[f(`railHeight`,e)]:v,[f(`railWidth`,e)]:y,[f(`railBorderRadius`,e)]:b,[f(`buttonBorderRadius`,e)]:x},common:{cubicBezierEaseInOut:S}}=a.value,C,w,T;return z?(C=`calc((${v} - ${p}) / 2)`,w=`max(${v}, ${p})`,T=`max(${y}, calc(${y} + ${p} - ${v}))`):(C=g((_(v)-_(p))/2),w=g(Math.max(_(v),_(p))),T=_(v)>_(p)?y:g(_(y)+_(p)-_(v))),{"--n-bezier":S,"--n-button-border-radius":x,"--n-button-box-shadow":i,"--n-button-color":o,"--n-button-width":m,"--n-button-width-pressed":h,"--n-button-height":p,"--n-height":w,"--n-offset":C,"--n-opacity-disabled":t,"--n-rail-border-radius":b,"--n-rail-color":n,"--n-rail-color-active":r,"--n-rail-height":v,"--n-rail-width":y,"--n-width":T,"--n-box-shadow-focus":s,"--n-loading-color":l,"--n-text-color":u,"--n-icon-color":d}}),R=r?s(`switch`,C(()=>c.value[0]),L,e):void 0;return{handleClick:O,handleBlur:j,handleFocus:A,handleKeyup:N,handleKeydown:P,mergedRailStyle:w,pressed:b,mergedClsPrefix:t,mergedValue:m,checked:v,mergedDisabled:l,cssVars:r?void 0:L,themeClass:R?.themeClass,onRender:R?.onRender}},render(){let{mergedClsPrefix:t,mergedDisabled:n,checked:r,mergedRailStyle:i,onRender:o,$slots:s}=this;o?.();let{checked:c,unchecked:u,icon:d,"checked-icon":f,"unchecked-icon":p}=s,h=!(l(d)&&l(f)&&l(p));return b(),T(`div`,{role:`switch`,"aria-checked":r,class:a([`${t}-switch`,this.themeClass,h&&`${t}-switch--icon`,r&&`${t}-switch--active`,n&&`${t}-switch--disabled`,this.round&&`${t}-switch--round`,this.loading&&`${t}-switch--loading`,this.pressed&&`${t}-switch--pressed`,this.rubberBand&&`${t}-switch--rubber-band`]),tabindex:this.mergedDisabled?void 0:0,style:x(this.cssVars),onClick:this.handleClick,onFocus:this.handleFocus,onBlur:this.handleBlur,onKeyup:this.handleKeyup,onKeydown:this.handleKeydown},[w(`div`,{class:a(`${t}-switch__rail`),"aria-hidden":`true`,style:x(i)},[m(()=>e(c,n=>e(u,e=>n||e?(b(),T(`div`,{key:4,"aria-hidden":!0,class:a(`${t}-switch__children-placeholder`)},[w(`div`,{class:a(`${t}-switch__rail-placeholder`)},[w(`div`,{class:a(`${t}-switch__button-placeholder`)},null,2),m(()=>n)],2),w(`div`,{class:a(`${t}-switch__rail-placeholder`)},[w(`div`,{class:a(`${t}-switch__button-placeholder`)},null,2),m(()=>e)],2)],2)):null))),w(`div`,{class:a(`${t}-switch__button`)},[m(()=>e(d,n=>e(f,r=>e(p,e=>(b(),D(A,null,{default:()=>this.loading?(b(),D(N,E({key:`loading`,clsPrefix:t,strokeWidth:20},this.spinProps),null,16,[`clsPrefix`])):this.checked&&(r||n)?(b(),T(`div`,{class:a(`${t}-switch__button-icon`),key:r?`checked-icon`:`icon`},[m(()=>r||n)],2)):!this.checked&&(e||n)?(b(),T(`div`,{class:a(`${t}-switch__button-icon`),key:e?`unchecked-icon`:`icon`},[m(()=>e||n)],2)):null},1024)))))),m(()=>e(c,e=>e&&(b(),T(`div`,{key:`checked`,class:a(`${t}-switch__checked`)},[m(()=>e)],2)))),m(()=>e(u,e=>e&&(b(),T(`div`,{key:`unchecked`,class:a(`${t}-switch__unchecked`)},[m(()=>e)],2))))],2)],6)],46,L)}});export{B as t};