import{D as e,S as t,a as n,at as r,c as i,ct as a,d as o,f as s,g as c,it as l,l as u,lt as d,n as f,st as p,u as m,ut as h,w as g,y as _}from"./resolve-slot-D0ESJhJD.js";import{n as v,t as y}from"./use-rtl-B41EZXdW.js";import{E as b,F as x,M as S,O as C,S as w,St as T,V as E,Wt as D,_t as O,at as k,b as A,d as j,f as M,h as N,i as P,j as F,p as I}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{n as L,t as R}from"./runtime-dom.esm-bundler-B3Pl3ttw.js";import{t as z}from"./use-style-CXm9Jcwk.js";import{i as ee,n as B,o as te,t as V}from"./icon-switch.cssr-o4i5UUsg.js";import{t as H}from"./color-to-class-G4jmrDIg.js";import{t as U}from"./is-browser-e-NGrvPW.js";var W=w({name:`FadeInExpandTransition`,props:{appear:Boolean,group:Boolean,mode:String,onLeave:Function,onAfterLeave:Function,onAfterEnter:Function,width:Boolean,reverse:Boolean},setup(e,{slots:t}){function n(t){e.width?t.style.maxWidth=`${t.offsetWidth}px`:t.style.maxHeight=`${t.offsetHeight}px`,t.offsetWidth}function r(t){e.width?t.style.maxWidth=`0`:t.style.maxHeight=`0`,t.offsetWidth;let{onLeave:n}=e;n&&n()}function i(t){e.width?t.style.maxWidth=``:t.style.maxHeight=``;let{onAfterLeave:n}=e;n&&n()}function a(t){if(t.style.transition=`none`,e.width){let e=t.offsetWidth;t.style.maxWidth=`0`,t.offsetWidth,t.style.transition=``,t.style.maxWidth=`${e}px`}else if(e.reverse)t.style.maxHeight=`${t.offsetHeight}px`,t.offsetHeight,t.style.transition=``,t.style.maxHeight=`0`;else{let e=t.offsetHeight;t.style.maxHeight=`0`,t.offsetWidth,t.style.transition=``,t.style.maxHeight=`${e}px`}t.offsetWidth}function o(t){e.width?t.style.maxWidth=``:e.reverse||(t.style.maxHeight=``),e.onAfterEnter?.()}return()=>{let{group:s,width:c,appear:l,mode:u}=e,d=s?L:R,f={name:c?`fade-in-width-expand-transition`:`fade-in-height-expand-transition`,appear:l,onEnter:a,onAfterEnter:o,onBeforeLeave:n,onLeave:r,onAfterLeave:i};return s||(f.mode=u),b(d,f,t)}}}),ne=l([l(`@keyframes rotator`,`
 0% {
 -webkit-transform: rotate(0deg);
 transform: rotate(0deg);
 }
 100% {
 -webkit-transform: rotate(360deg);
 transform: rotate(360deg);
 }`),r(`base-loading`,`
 position: relative;
 line-height: 0;
 width: 1em;
 height: 1em;
 `,[p(`transition-wrapper`,`
 position: absolute;
 width: 100%;
 height: 100%;
 `,[V()]),p(`placeholder`,`
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 `,[V({left:`50%`,top:`50%`,originalTransform:`translateX(-50%) translateY(-50%)`})]),p(`container`,`
 animation: rotator 3s linear infinite both;
 `,[p(`icon`,`
 height: 1em;
 width: 1em;
 `)])])]),re=[`viewBox`],ie=[`values`,`dur`],ae=[`stroke-width`,`cx`,`cy`,`r`,`stroke-dasharray`,`stroke-dashoffset`],oe=[`values`,`dur`],se=[`values`,`dur`],G=`1.6s`,K={strokeWidth:{type:Number,default:28},stroke:{type:String,default:void 0},scale:{type:Number,default:1},radius:{type:Number,default:100}},q=w({name:`BaseLoading`,props:{clsPrefix:{type:String,required:!0},show:{type:Boolean,default:!0},...K},setup(e){z(`-base-loading`,ne,T(e,`clsPrefix`))},render(){let{clsPrefix:e,radius:t,strokeWidth:n,stroke:r,scale:i}=this,a=t/i;return E(),N(`div`,{class:c(`${e}-base-loading`),role:`img`,"aria-label":`loading`},[A(B,null,{default:()=>this.show?(E(),N(`div`,{key:`icon`,class:c(`${e}-base-loading__transition-wrapper`)},[M(`div`,{class:c(`${e}-base-loading__container`)},[(E(),N(`svg`,{class:c(`${e}-base-loading__icon`),viewBox:`0 0 ${2*a} ${2*a}`,xmlns:`http://www.w3.org/2000/svg`,style:D({color:r})},[M(`g`,null,[M(`animateTransform`,{attributeName:`transform`,type:`rotate`,values:`0 ${a} ${a};270 ${a} ${a}`,begin:`0s`,dur:G,fill:`freeze`,repeatCount:`indefinite`},null,8,ie),M(`circle`,{class:c(`${e}-base-loading__icon`),fill:`none`,stroke:`currentColor`,"stroke-width":n,"stroke-linecap":`round`,cx:a,cy:a,r:t-n/2,"stroke-dasharray":5.67*t,"stroke-dashoffset":18.48*t},[M(`animateTransform`,{attributeName:`transform`,type:`rotate`,values:`0 ${a} ${a};135 ${a} ${a};450 ${a} ${a}`,begin:`0s`,dur:G,fill:`freeze`,repeatCount:`indefinite`},null,8,oe),M(`animate`,{attributeName:`stroke-dashoffset`,values:`${5.67*t};${1.42*t};${5.67*t}`,begin:`0s`,dur:G,fill:`freeze`,repeatCount:`indefinite`},null,8,se)],10,ae)])],14,re))],2)],2)):(E(),N(`div`,{key:`placeholder`,class:c(`${e}-base-loading__placeholder`)},[_(()=>this.$slots.default?.())],2))},1024)],2)}}),ce=U&&`chrome`in window;U&&navigator.userAgent.includes(`Firefox`);var J=U&&navigator.userAgent.includes(`Safari`)&&!ce,{cubicBezierEaseInOut:Y}=t;function X({duration:e=`.2s`,delay:t=`.1s`}={}){return[l(`&.fade-in-width-expand-transition-leave-from, &.fade-in-width-expand-transition-enter-to`,{opacity:1}),l(`&.fade-in-width-expand-transition-leave-to, &.fade-in-width-expand-transition-enter-from`,`
 opacity: 0!important;
 margin-left: 0!important;
 margin-right: 0!important;
 `),l(`&.fade-in-width-expand-transition-leave-active`,`
 overflow: hidden;
 transition:
 opacity ${e} ${Y},
 max-width ${e} ${Y} ${t},
 margin-left ${e} ${Y} ${t},
 margin-right ${e} ${Y} ${t};
 `),l(`&.fade-in-width-expand-transition-enter-active`,`
 overflow: hidden;
 transition:
 opacity ${e} ${Y} ${t},
 max-width ${e} ${Y},
 margin-left ${e} ${Y},
 margin-right ${e} ${Y};
 `)]}var le=r(`base-wave`,`
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 border-radius: inherit;
`),Z=w({name:`BaseWave`,props:{clsPrefix:{type:String,required:!0}},setup(e){z(`-base-wave`,le,T(e,`clsPrefix`));let t=O(null),n=O(!1),r=null;return x(()=>{r!==null&&window.clearTimeout(r)}),{active:n,selfRef:t,play(){r!==null&&(window.clearTimeout(r),n.value=!1,r=null),S(()=>{t.value?.offsetHeight,n.value=!0,r=window.setTimeout(()=>{n.value=!1,r=null},1e3)})}}},render(){let{clsPrefix:e}=this;return E(),N(`div`,{ref:`selfRef`,"aria-hidden":!0,class:c([`${e}-base-wave`,this.active&&`${e}-base-wave--active`])},null,2)}}),ue={paddingTiny:`0 6px`,paddingSmall:`0 10px`,paddingMedium:`0 14px`,paddingLarge:`0 18px`,paddingRoundTiny:`0 10px`,paddingRoundSmall:`0 14px`,paddingRoundMedium:`0 18px`,paddingRoundLarge:`0 22px`,iconMarginTiny:`6px`,iconMarginSmall:`6px`,iconMarginMedium:`6px`,iconMarginLarge:`6px`,iconSizeTiny:`14px`,iconSizeSmall:`18px`,iconSizeMedium:`18px`,iconSizeLarge:`20px`,rippleDuration:`.6s`};function de(e){let{heightTiny:t,heightSmall:n,heightMedium:r,heightLarge:i,borderRadius:a,fontSizeTiny:o,fontSizeSmall:s,fontSizeMedium:c,fontSizeLarge:l,opacityDisabled:u,textColor2:d,textColor3:f,primaryColorHover:p,primaryColorPressed:m,borderColor:h,primaryColor:g,baseColor:_,infoColor:v,infoColorHover:y,infoColorPressed:b,successColor:x,successColorHover:S,successColorPressed:C,warningColor:w,warningColorHover:T,warningColorPressed:E,errorColor:D,errorColorHover:O,errorColorPressed:k,fontWeight:A,buttonColor2:j,buttonColor2Hover:M,buttonColor2Pressed:N,fontWeightStrong:P}=e;return{...ue,heightTiny:t,heightSmall:n,heightMedium:r,heightLarge:i,borderRadiusTiny:a,borderRadiusSmall:a,borderRadiusMedium:a,borderRadiusLarge:a,fontSizeTiny:o,fontSizeSmall:s,fontSizeMedium:c,fontSizeLarge:l,opacityDisabled:u,colorOpacitySecondary:`0.16`,colorOpacitySecondaryHover:`0.22`,colorOpacitySecondaryPressed:`0.28`,colorSecondary:j,colorSecondaryHover:M,colorSecondaryPressed:N,colorTertiary:j,colorTertiaryHover:M,colorTertiaryPressed:N,colorQuaternary:`#0000`,colorQuaternaryHover:M,colorQuaternaryPressed:N,color:`#0000`,colorHover:`#0000`,colorPressed:`#0000`,colorFocus:`#0000`,colorDisabled:`#0000`,textColor:d,textColorTertiary:f,textColorHover:p,textColorPressed:m,textColorFocus:p,textColorDisabled:d,textColorText:d,textColorTextHover:p,textColorTextPressed:m,textColorTextFocus:p,textColorTextDisabled:d,textColorGhost:d,textColorGhostHover:p,textColorGhostPressed:m,textColorGhostFocus:p,textColorGhostDisabled:d,border:`1px solid ${h}`,borderHover:`1px solid ${p}`,borderPressed:`1px solid ${m}`,borderFocus:`1px solid ${p}`,borderDisabled:`1px solid ${h}`,rippleColor:g,colorPrimary:g,colorHoverPrimary:p,colorPressedPrimary:m,colorFocusPrimary:p,colorDisabledPrimary:g,textColorPrimary:_,textColorHoverPrimary:_,textColorPressedPrimary:_,textColorFocusPrimary:_,textColorDisabledPrimary:_,textColorTextPrimary:g,textColorTextHoverPrimary:p,textColorTextPressedPrimary:m,textColorTextFocusPrimary:p,textColorTextDisabledPrimary:d,textColorGhostPrimary:g,textColorGhostHoverPrimary:p,textColorGhostPressedPrimary:m,textColorGhostFocusPrimary:p,textColorGhostDisabledPrimary:g,borderPrimary:`1px solid ${g}`,borderHoverPrimary:`1px solid ${p}`,borderPressedPrimary:`1px solid ${m}`,borderFocusPrimary:`1px solid ${p}`,borderDisabledPrimary:`1px solid ${g}`,rippleColorPrimary:g,colorInfo:v,colorHoverInfo:y,colorPressedInfo:b,colorFocusInfo:y,colorDisabledInfo:v,textColorInfo:_,textColorHoverInfo:_,textColorPressedInfo:_,textColorFocusInfo:_,textColorDisabledInfo:_,textColorTextInfo:v,textColorTextHoverInfo:y,textColorTextPressedInfo:b,textColorTextFocusInfo:y,textColorTextDisabledInfo:d,textColorGhostInfo:v,textColorGhostHoverInfo:y,textColorGhostPressedInfo:b,textColorGhostFocusInfo:y,textColorGhostDisabledInfo:v,borderInfo:`1px solid ${v}`,borderHoverInfo:`1px solid ${y}`,borderPressedInfo:`1px solid ${b}`,borderFocusInfo:`1px solid ${y}`,borderDisabledInfo:`1px solid ${v}`,rippleColorInfo:v,colorSuccess:x,colorHoverSuccess:S,colorPressedSuccess:C,colorFocusSuccess:S,colorDisabledSuccess:x,textColorSuccess:_,textColorHoverSuccess:_,textColorPressedSuccess:_,textColorFocusSuccess:_,textColorDisabledSuccess:_,textColorTextSuccess:x,textColorTextHoverSuccess:S,textColorTextPressedSuccess:C,textColorTextFocusSuccess:S,textColorTextDisabledSuccess:d,textColorGhostSuccess:x,textColorGhostHoverSuccess:S,textColorGhostPressedSuccess:C,textColorGhostFocusSuccess:S,textColorGhostDisabledSuccess:x,borderSuccess:`1px solid ${x}`,borderHoverSuccess:`1px solid ${S}`,borderPressedSuccess:`1px solid ${C}`,borderFocusSuccess:`1px solid ${S}`,borderDisabledSuccess:`1px solid ${x}`,rippleColorSuccess:x,colorWarning:w,colorHoverWarning:T,colorPressedWarning:E,colorFocusWarning:T,colorDisabledWarning:w,textColorWarning:_,textColorHoverWarning:_,textColorPressedWarning:_,textColorFocusWarning:_,textColorDisabledWarning:_,textColorTextWarning:w,textColorTextHoverWarning:T,textColorTextPressedWarning:E,textColorTextFocusWarning:T,textColorTextDisabledWarning:d,textColorGhostWarning:w,textColorGhostHoverWarning:T,textColorGhostPressedWarning:E,textColorGhostFocusWarning:T,textColorGhostDisabledWarning:w,borderWarning:`1px solid ${w}`,borderHoverWarning:`1px solid ${T}`,borderPressedWarning:`1px solid ${E}`,borderFocusWarning:`1px solid ${T}`,borderDisabledWarning:`1px solid ${w}`,rippleColorWarning:w,colorError:D,colorHoverError:O,colorPressedError:k,colorFocusError:O,colorDisabledError:D,textColorError:_,textColorHoverError:_,textColorPressedError:_,textColorFocusError:_,textColorDisabledError:_,textColorTextError:D,textColorTextHoverError:O,textColorTextPressedError:k,textColorTextFocusError:O,textColorTextDisabledError:d,textColorGhostError:D,textColorGhostHoverError:O,textColorGhostPressedError:k,textColorGhostFocusError:O,textColorGhostDisabledError:D,borderError:`1px solid ${D}`,borderHoverError:`1px solid ${O}`,borderPressedError:`1px solid ${k}`,borderFocusError:`1px solid ${O}`,borderDisabledError:`1px solid ${D}`,rippleColorError:D,waveOpacity:`0.6`,fontWeight:A,fontWeightStrong:P}}var fe={name:`Button`,common:m,self:de};function Q(e){return s(e,[255,255,255,.16])}function $(e){return s(e,[0,0,0,.12])}var pe=e(`n-button-group`),me=l([r(`button`,`
 margin: 0;
 font-weight: var(--n-font-weight);
 line-height: 1;
 font-family: inherit;
 padding: var(--n-padding);
 height: var(--n-height);
 font-size: var(--n-font-size);
 border-radius: var(--n-border-radius);
 color: var(--n-text-color);
 background-color: var(--n-color);
 width: var(--n-width);
 white-space: nowrap;
 outline: none;
 position: relative;
 z-index: auto;
 border: none;
 display: inline-flex;
 flex-wrap: nowrap;
 flex-shrink: 0;
 align-items: center;
 justify-content: center;
 user-select: none;
 -webkit-user-select: none;
 text-align: center;
 cursor: pointer;
 text-decoration: none;
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 opacity .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `,[a(`color`,[p(`border`,{borderColor:`var(--n-border-color)`}),a(`disabled`,[p(`border`,{borderColor:`var(--n-border-color-disabled)`})]),d(`disabled`,[l(`&:focus`,[p(`state-border`,{borderColor:`var(--n-border-color-focus)`})]),l(`&:hover`,[p(`state-border`,{borderColor:`var(--n-border-color-hover)`})]),l(`&:active`,[p(`state-border`,{borderColor:`var(--n-border-color-pressed)`})]),a(`pressed`,[p(`state-border`,{borderColor:`var(--n-border-color-pressed)`})])])]),a(`disabled`,{backgroundColor:`var(--n-color-disabled)`,color:`var(--n-text-color-disabled)`},[p(`border`,{border:`var(--n-border-disabled)`})]),d(`disabled`,[l(`&:focus`,{backgroundColor:`var(--n-color-focus)`,color:`var(--n-text-color-focus)`},[p(`state-border`,{border:`var(--n-border-focus)`})]),l(`&:hover`,{backgroundColor:`var(--n-color-hover)`,color:`var(--n-text-color-hover)`},[p(`state-border`,{border:`var(--n-border-hover)`})]),l(`&:active`,{backgroundColor:`var(--n-color-pressed)`,color:`var(--n-text-color-pressed)`},[p(`state-border`,{border:`var(--n-border-pressed)`})]),a(`pressed`,{backgroundColor:`var(--n-color-pressed)`,color:`var(--n-text-color-pressed)`},[p(`state-border`,{border:`var(--n-border-pressed)`})])]),a(`loading`,`cursor: wait;`),r(`base-wave`,`
 pointer-events: none;
 top: 0;
 right: 0;
 bottom: 0;
 left: 0;
 animation-iteration-count: 1;
 animation-duration: var(--n-ripple-duration);
 animation-timing-function: var(--n-bezier-ease-out), var(--n-bezier-ease-out);
 `,[a(`active`,{zIndex:1,animationName:`button-wave-spread, button-wave-opacity`})]),U&&`MozBoxSizing`in document.createElement(`div`).style?l(`&::moz-focus-inner`,{border:0}):null,p(`border, state-border`,`
 position: absolute;
 left: 0;
 top: 0;
 right: 0;
 bottom: 0;
 border-radius: inherit;
 transition: border-color .3s var(--n-bezier);
 pointer-events: none;
 `),p(`border`,`
 border: var(--n-border);
 `),p(`state-border`,`
 border: var(--n-border);
 border-color: #0000;
 z-index: 1;
 `),p(`icon`,`
 margin: var(--n-icon-margin);
 margin-left: 0;
 height: var(--n-icon-size);
 width: var(--n-icon-size);
 max-width: var(--n-icon-size);
 font-size: var(--n-icon-size);
 position: relative;
 flex-shrink: 0;
 `,[r(`icon-slot`,`
 height: var(--n-icon-size);
 width: var(--n-icon-size);
 position: absolute;
 left: 0;
 top: 50%;
 transform: translateY(-50%);
 display: flex;
 align-items: center;
 justify-content: center;
 `,[V({top:`50%`,originalTransform:`translateY(-50%)`})]),X()]),p(`content`,`
 display: flex;
 align-items: center;
 flex-wrap: nowrap;
 min-width: 0;
 `,[l(`~`,[p(`icon`,{margin:`var(--n-icon-margin)`,marginRight:0})])]),a(`block`,`
 display: flex;
 width: 100%;
 `),a(`dashed`,[p(`border, state-border`,{borderStyle:`dashed !important`})]),a(`disabled`,{cursor:`not-allowed`,opacity:`var(--n-opacity-disabled)`})]),l(`@keyframes button-wave-spread`,{from:{boxShadow:`0 0 0.5px 0 var(--n-ripple-color)`},to:{boxShadow:`0 0 0.5px 4.5px var(--n-ripple-color)`}}),l(`@keyframes button-wave-opacity`,{from:{opacity:`var(--n-wave-opacity)`},to:{opacity:0}})]),he={...i.props,color:String,textColor:String,text:Boolean,block:Boolean,loading:Boolean,disabled:Boolean,circle:Boolean,size:String,ghost:Boolean,round:Boolean,secondary:Boolean,tertiary:Boolean,quaternary:Boolean,strong:Boolean,focusable:{type:Boolean,default:!0},keyboard:{type:Boolean,default:!0},tag:{type:String,default:`button`},type:{type:String,default:`default`},dashed:Boolean,renderIcon:Function,iconPlacement:{type:String,default:`left`},attrType:{type:String,default:`button`},bordered:{type:Boolean,default:!0},onClick:[Function,Array],nativeFocusBehavior:{type:Boolean,default:!J},spinProps:Object},ge=w({name:`Button`,props:he,slots:Object,setup(e){let t=O(null),n=O(null),r=O(!1),a=te(()=>!e.quaternary&&!e.tertiary&&!e.secondary&&!e.text&&(!e.color||e.ghost||e.dashed)&&e.bordered),s=C(pe,{}),{inlineThemeDisabled:c,mergedClsPrefixRef:l,mergedRtlRef:d,mergedComponentPropsRef:f}=g(e),{mergedSizeRef:p}=ee({},{defaultSize:`medium`,mergedSize:t=>{let{size:n}=e;if(n)return n;let{size:r}=s;if(r)return r;let{mergedSize:i}=t||{};return i?i.value:f?.value?.Button?.size||`medium`}}),m=j(()=>e.focusable&&!e.disabled),_=n=>{m.value||n.preventDefault(),!e.nativeFocusBehavior&&(n.preventDefault(),!e.disabled&&m.value&&t.value?.focus({preventScroll:!0}))},b=t=>{if(!e.disabled&&!e.loading){let{onClick:r}=e;r&&v(r,t),e.text||n.value?.play()}},x=t=>{if(t.key===`Enter`){if(!e.keyboard)return;r.value=!1}},S=t=>{if(t.key===`Enter`){if(!e.keyboard||e.loading){t.preventDefault();return}r.value=!0}},w=()=>{r.value=!1},T=i(`Button`,`-button`,me,fe,e,l),E=y(`Button`,d,l),D=j(()=>{let{common:{cubicBezierEaseInOut:t,cubicBezierEaseOut:n},self:r}=T.value,{rippleDuration:i,opacityDisabled:a,fontWeight:s,fontWeightStrong:c}=r,l=p.value,{dashed:u,type:d,ghost:f,text:m,color:g,round:_,circle:v,textColor:y,secondary:b,tertiary:x,quaternary:S,strong:C}=e,w={"--n-font-weight":C?c:s},E={"--n-color":`initial`,"--n-color-hover":`initial`,"--n-color-pressed":`initial`,"--n-color-focus":`initial`,"--n-color-disabled":`initial`,"--n-ripple-color":`initial`,"--n-text-color":`initial`,"--n-text-color-hover":`initial`,"--n-text-color-pressed":`initial`,"--n-text-color-focus":`initial`,"--n-text-color-disabled":`initial`},D=d===`tertiary`,O=d==="default",k=D?`default`:d;if(m){let e=y||g;E={"--n-color":`#0000`,"--n-color-hover":`#0000`,"--n-color-pressed":`#0000`,"--n-color-focus":`#0000`,"--n-color-disabled":`#0000`,"--n-ripple-color":`#0000`,"--n-text-color":e||r[h(`textColorText`,k)],"--n-text-color-hover":e?Q(e):r[h(`textColorTextHover`,k)],"--n-text-color-pressed":e?$(e):r[h(`textColorTextPressed`,k)],"--n-text-color-focus":e?Q(e):r[h(`textColorTextHover`,k)],"--n-text-color-disabled":e||r[h(`textColorTextDisabled`,k)]}}else if(f||u){let e=y||g;E={"--n-color":`#0000`,"--n-color-hover":`#0000`,"--n-color-pressed":`#0000`,"--n-color-focus":`#0000`,"--n-color-disabled":`#0000`,"--n-ripple-color":g||r[h(`rippleColor`,k)],"--n-text-color":e||r[h(`textColorGhost`,k)],"--n-text-color-hover":e?Q(e):r[h(`textColorGhostHover`,k)],"--n-text-color-pressed":e?$(e):r[h(`textColorGhostPressed`,k)],"--n-text-color-focus":e?Q(e):r[h(`textColorGhostHover`,k)],"--n-text-color-disabled":e||r[h(`textColorGhostDisabled`,k)]}}else if(b){let e=O?r.textColor:D?r.textColorTertiary:r[h(`color`,k)],t=g||e,n=d!=="default"&&d!==`tertiary`;E={"--n-color":n?o(t,{alpha:Number(r.colorOpacitySecondary)}):r.colorSecondary,"--n-color-hover":n?o(t,{alpha:Number(r.colorOpacitySecondaryHover)}):r.colorSecondaryHover,"--n-color-pressed":n?o(t,{alpha:Number(r.colorOpacitySecondaryPressed)}):r.colorSecondaryPressed,"--n-color-focus":n?o(t,{alpha:Number(r.colorOpacitySecondaryHover)}):r.colorSecondaryHover,"--n-color-disabled":r.colorSecondary,"--n-ripple-color":`#0000`,"--n-text-color":t,"--n-text-color-hover":t,"--n-text-color-pressed":t,"--n-text-color-focus":t,"--n-text-color-disabled":t}}else if(x||S){let e=O?r.textColor:D?r.textColorTertiary:r[h(`color`,k)],t=g||e;x?(E[`--n-color`]=r.colorTertiary,E[`--n-color-hover`]=r.colorTertiaryHover,E[`--n-color-pressed`]=r.colorTertiaryPressed,E[`--n-color-focus`]=r.colorSecondaryHover,E[`--n-color-disabled`]=r.colorTertiary):(E[`--n-color`]=r.colorQuaternary,E[`--n-color-hover`]=r.colorQuaternaryHover,E[`--n-color-pressed`]=r.colorQuaternaryPressed,E[`--n-color-focus`]=r.colorQuaternaryHover,E[`--n-color-disabled`]=r.colorQuaternary),E[`--n-ripple-color`]=`#0000`,E[`--n-text-color`]=t,E[`--n-text-color-hover`]=t,E[`--n-text-color-pressed`]=t,E[`--n-text-color-focus`]=t,E[`--n-text-color-disabled`]=t}else E={"--n-color":g||r[h(`color`,k)],"--n-color-hover":g?Q(g):r[h(`colorHover`,k)],"--n-color-pressed":g?$(g):r[h(`colorPressed`,k)],"--n-color-focus":g?Q(g):r[h(`colorFocus`,k)],"--n-color-disabled":g||r[h(`colorDisabled`,k)],"--n-ripple-color":g||r[h(`rippleColor`,k)],"--n-text-color":y||(g?r.textColorPrimary:D?r.textColorTertiary:r[h(`textColor`,k)]),"--n-text-color-hover":y||(g?r.textColorHoverPrimary:r[h(`textColorHover`,k)]),"--n-text-color-pressed":y||(g?r.textColorPressedPrimary:r[h(`textColorPressed`,k)]),"--n-text-color-focus":y||(g?r.textColorFocusPrimary:r[h(`textColorFocus`,k)]),"--n-text-color-disabled":y||(g?r.textColorDisabledPrimary:r[h(`textColorDisabled`,k)])};let A={"--n-border":`initial`,"--n-border-hover":`initial`,"--n-border-pressed":`initial`,"--n-border-focus":`initial`,"--n-border-disabled":`initial`};A=m?{"--n-border":`none`,"--n-border-hover":`none`,"--n-border-pressed":`none`,"--n-border-focus":`none`,"--n-border-disabled":`none`}:{"--n-border":r[h(`border`,k)],"--n-border-hover":r[h(`borderHover`,k)],"--n-border-pressed":r[h(`borderPressed`,k)],"--n-border-focus":r[h(`borderFocus`,k)],"--n-border-disabled":r[h(`borderDisabled`,k)]};let{[h(`height`,l)]:j,[h(`fontSize`,l)]:M,[h(`padding`,l)]:N,[h(`paddingRound`,l)]:P,[h(`iconSize`,l)]:F,[h(`borderRadius`,l)]:I,[h(`iconMargin`,l)]:L,waveOpacity:R}=r,z={"--n-width":v&&!m?j:`initial`,"--n-height":m?`initial`:j,"--n-font-size":M,"--n-padding":v||m?`initial`:_?P:N,"--n-icon-size":F,"--n-icon-margin":L,"--n-border-radius":m?`initial`:v||_?j:I};return{"--n-bezier":t,"--n-bezier-ease-out":n,"--n-ripple-duration":i,"--n-opacity-disabled":a,"--n-wave-opacity":R,...w,...E,...A,...z}}),k=c?u(`button`,j(()=>{let t=``,{dashed:n,type:r,ghost:i,text:a,color:o,round:s,circle:c,textColor:l,secondary:u,tertiary:d,quaternary:f,strong:m}=e;n&&(t+=`a`),i&&(t+=`b`),a&&(t+=`c`),s&&(t+=`d`),c&&(t+=`e`),u&&(t+=`f`),d&&(t+=`g`),f&&(t+=`h`),m&&(t+=`i`),o&&(t+=`j${H(o)}`),l&&(t+=`k${H(l)}`);let{value:h}=p;return t+=`l${h[0]}`,t+=`m${r[0]}`,t}),D,e):void 0;return{selfElRef:t,waveElRef:n,mergedClsPrefix:l,mergedFocusable:m,mergedSize:p,showBorder:a,enterPressed:r,rtlEnabled:E,handleMousedown:_,handleKeydown:S,handleBlur:w,handleKeyup:x,handleClick:b,customColorCssVars:j(()=>{let{color:t}=e;if(!t)return null;let n=Q(t);return{"--n-border-color":t,"--n-border-color-hover":n,"--n-border-color-pressed":$(t),"--n-border-color-focus":n,"--n-border-color-disabled":t}}),cssVars:c?void 0:D,themeClass:k?.themeClass,onRender:k?.onRender}},render(){let{mergedClsPrefix:e,tag:t,onRender:r}=this;r?.();let i=n(this.$slots.default,t=>t&&(E(),N(`span`,{class:c(`${e}-button__content`)},[_(()=>t)],2)));return E(),I(t,{ref:`selfElRef`,class:c([this.themeClass,`${e}-button`,`${e}-button--${this.type}-type`,`${e}-button--${this.mergedSize}-type`,this.rtlEnabled&&`${e}-button--rtl`,this.disabled&&`${e}-button--disabled`,this.block&&`${e}-button--block`,this.enterPressed&&`${e}-button--pressed`,!this.text&&this.dashed&&`${e}-button--dashed`,this.color&&`${e}-button--color`,this.secondary&&`${e}-button--secondary`,this.loading&&`${e}-button--loading`,this.ghost&&`${e}-button--ghost`]),tabindex:this.mergedFocusable?0:-1,type:this.attrType,style:D(this.cssVars),disabled:this.disabled,onClick:this.handleClick,onBlur:this.handleBlur,onMousedown:this.handleMousedown,onKeyup:this.handleKeyup,onKeydown:this.handleKeydown},{default:k(()=>[_(()=>this.iconPlacement===`right`&&i),A(W,{width:!0},{default:()=>n(this.$slots.icon,t=>(this.loading||this.renderIcon||t)&&(E(),N(`span`,{class:c(`${e}-button__icon`),style:D({margin:f(this.$slots.default)?`0`:``})},[A(B,null,{default:()=>this.loading?(E(),I(q,F({clsPrefix:e,key:`loading`,class:`${e}-icon-slot`,strokeWidth:20},this.spinProps),null,16,[`clsPrefix`,`class`])):(E(),N(`div`,{key:`icon`,class:c(`${e}-icon-slot`),role:`none`},[this.renderIcon?(E(),N(P,{key:0},[_(()=>this.renderIcon())],64)):(E(),N(P,{key:1},[_(()=>t)],64))],2))},1024)],6)))},1024),_(()=>this.iconPlacement===`left`&&i),this.text?_(()=>null):(E(),I(Z,{key:0,ref:`waveElRef`,clsPrefix:e},null,8,[`clsPrefix`])),this.showBorder?(E(),N(`div`,{key:2,"aria-hidden":!0,class:c(`${e}-button__border`),style:D(this.customColorCssVars)},null,6)):_(()=>null),this.showBorder?(E(),N(`div`,{key:4,"aria-hidden":!0,class:c(`${e}-button__state-border`),style:D(this.customColorCssVars)},null,6)):_(()=>null)]),_:2},1032,[`class`,`tabindex`,`type`,`style`,`disabled`,`onClick`,`onBlur`,`onMousedown`,`onKeyup`,`onKeydown`])}}),_e=ge;export{de as a,J as c,W as d,fe as i,q as l,_e as n,Z as o,pe as r,X as s,ge as t,K as u};