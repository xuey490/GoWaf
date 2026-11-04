import{a as e,at as t,c as n,ct as r,d as i,f as a,g as o,it as s,l as c,r as l,st as u,u as d,ut as f,w as p,y as m}from"./resolve-slot-D0ESJhJD.js";import{t as h}from"./use-rtl-B41EZXdW.js";import{Ot as g}from"./user-BFrG88cX.js";import{a as _,n as v}from"./replaceable-CG1Qs4lR.js";import{S as y,V as b,_t as x,d as S,f as C,h as w,j as T,p as E}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{p as D}from"./vue-i18n-D4LRfH1J.js";import{t as O}from"./Close-swctlKIK.js";import{i as k,n as A,r as j,t as M}from"./Warning-DwQXxd7L.js";import{d as N}from"./Button-DUNxlS0h.js";function P(e){let{lineHeight:t,borderRadius:n,fontWeightStrong:r,baseColor:o,dividerColor:s,actionColor:c,textColor1:l,textColor2:u,closeColorHover:d,closeColorPressed:f,closeIconColor:p,closeIconColorHover:m,closeIconColorPressed:h,infoColor:_,successColor:v,warningColor:y,errorColor:b,fontSize:x}=e;return{...g,fontSize:x,lineHeight:t,titleFontWeight:r,borderRadius:n,border:`1px solid ${s}`,color:c,titleTextColor:l,iconColor:u,contentTextColor:u,closeBorderRadius:n,closeColorHover:d,closeColorPressed:f,closeIconColor:p,closeIconColorHover:m,closeIconColorPressed:h,borderInfo:`1px solid ${a(o,i(_,{alpha:.25}))}`,colorInfo:a(o,i(_,{alpha:.08})),titleTextColorInfo:l,iconColorInfo:_,contentTextColorInfo:u,closeColorHoverInfo:d,closeColorPressedInfo:f,closeIconColorInfo:p,closeIconColorHoverInfo:m,closeIconColorPressedInfo:h,borderSuccess:`1px solid ${a(o,i(v,{alpha:.25}))}`,colorSuccess:a(o,i(v,{alpha:.08})),titleTextColorSuccess:l,iconColorSuccess:v,contentTextColorSuccess:u,closeColorHoverSuccess:d,closeColorPressedSuccess:f,closeIconColorSuccess:p,closeIconColorHoverSuccess:m,closeIconColorPressedSuccess:h,borderWarning:`1px solid ${a(o,i(y,{alpha:.33}))}`,colorWarning:a(o,i(y,{alpha:.08})),titleTextColorWarning:l,iconColorWarning:y,contentTextColorWarning:u,closeColorHoverWarning:d,closeColorPressedWarning:f,closeIconColorWarning:p,closeIconColorHoverWarning:m,closeIconColorPressedWarning:h,borderError:`1px solid ${a(o,i(b,{alpha:.25}))}`,colorError:a(o,i(b,{alpha:.08})),titleTextColorError:l,iconColorError:b,contentTextColorError:u,closeColorHoverError:d,closeColorPressedError:f,closeIconColorError:p,closeIconColorHoverError:m,closeIconColorPressedError:h}}var F={name:`Alert`,common:d,self:P},I=t(`alert`,`
 line-height: var(--n-line-height);
 border-radius: var(--n-border-radius);
 position: relative;
 transition: background-color .3s var(--n-bezier);
 background-color: var(--n-color);
 text-align: start;
 word-break: break-word;
`,[u(`border`,`
 border-radius: inherit;
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 transition: border-color .3s var(--n-bezier);
 border: var(--n-border);
 pointer-events: none;
 `),r(`closable`,[t(`alert-body`,[u(`title`,`
 padding-right: 24px;
 `)])]),u(`icon`,{color:`var(--n-icon-color)`}),t(`alert-body`,{padding:`var(--n-padding)`},[u(`title`,{color:`var(--n-title-text-color)`}),u(`content`,{color:`var(--n-content-text-color)`})]),D({originalTransition:`transform .3s var(--n-bezier)`,enterToProps:{transform:`scale(1)`},leaveToProps:{transform:`scale(0.9)`}}),u(`icon`,`
 position: absolute;
 left: 0;
 top: 0;
 align-items: center;
 justify-content: center;
 display: flex;
 width: var(--n-icon-size);
 height: var(--n-icon-size);
 font-size: var(--n-icon-size);
 margin: var(--n-icon-margin);
 `),u(`close`,`
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 position: absolute;
 right: 0;
 top: 0;
 margin: var(--n-close-margin);
 `),r(`show-icon`,[t(`alert-body`,{paddingLeft:`calc(var(--n-icon-margin-left) + var(--n-icon-size) + var(--n-icon-margin-right))`})]),r(`right-adjust`,[t(`alert-body`,{paddingRight:`calc(var(--n-close-size) + var(--n-padding) + 2px)`})]),t(`alert-body`,`
 border-radius: var(--n-border-radius);
 transition: border-color .3s var(--n-bezier);
 `,[u(`title`,`
 transition: color .3s var(--n-bezier);
 font-size: 16px;
 line-height: 19px;
 font-weight: var(--n-title-font-weight);
 `,[s(`& +`,[u(`content`,{marginTop:`9px`})])]),u(`content`,{transition:`color .3s var(--n-bezier)`,fontSize:`var(--n-font-size)`})]),u(`icon`,{transition:`color .3s var(--n-bezier)`})]),L={...n.props,title:String,showIcon:{type:Boolean,default:!0},type:{type:String,default:`default`},bordered:{type:Boolean,default:!0},closable:Boolean,onClose:Function,onAfterLeave:Function,onAfterHide:Function},R=y({name:`Alert`,inheritAttrs:!1,props:L,slots:Object,setup(e){let{mergedClsPrefixRef:t,mergedBorderedRef:r,inlineThemeDisabled:i,mergedRtlRef:a}=p(e),o=n(`Alert`,`-alert`,I,F,e,t),s=h(`Alert`,a,t),l=S(()=>{let{common:{cubicBezierEaseInOut:t},self:n}=o.value,{fontSize:r,borderRadius:i,titleFontWeight:a,lineHeight:s,iconSize:c,iconMargin:l,iconMarginRtl:u,closeIconSize:d,closeBorderRadius:p,closeSize:m,closeMargin:h,closeMarginRtl:g,padding:v}=n,{type:y}=e,{left:b,right:x}=_(l);return{"--n-bezier":t,"--n-color":n[f(`color`,y)],"--n-close-icon-size":d,"--n-close-border-radius":p,"--n-close-color-hover":n[f(`closeColorHover`,y)],"--n-close-color-pressed":n[f(`closeColorPressed`,y)],"--n-close-icon-color":n[f(`closeIconColor`,y)],"--n-close-icon-color-hover":n[f(`closeIconColorHover`,y)],"--n-close-icon-color-pressed":n[f(`closeIconColorPressed`,y)],"--n-icon-color":n[f(`iconColor`,y)],"--n-border":n[f(`border`,y)],"--n-title-text-color":n[f(`titleTextColor`,y)],"--n-content-text-color":n[f(`contentTextColor`,y)],"--n-line-height":s,"--n-border-radius":i,"--n-font-size":r,"--n-title-font-weight":a,"--n-icon-size":c,"--n-icon-margin":l,"--n-icon-margin-rtl":u,"--n-close-size":m,"--n-close-margin":h,"--n-close-margin-rtl":g,"--n-padding":v,"--n-icon-margin-left":b,"--n-icon-margin-right":x}}),u=i?c(`alert`,S(()=>e.type[0]),l,e):void 0,d=x(!0),m=()=>{let{onAfterLeave:t,onAfterHide:n}=e;t&&t(),n&&n()};return{rtlEnabled:s,mergedClsPrefix:t,mergedBordered:r,visible:d,handleCloseClick:()=>{Promise.resolve(e.onClose?.()).then(e=>{e!==!1&&(d.value=!1)})},handleAfterLeave:()=>{m()},mergedTheme:o,cssVars:i?void 0:l,themeClass:u?.themeClass,onRender:u?.onRender}},render(){return this.onRender?.(),b(),E(N,{onAfterLeave:this.handleAfterLeave},{default:()=>{let{mergedClsPrefix:t,$slots:n}=this,r={class:[`${t}-alert`,this.themeClass,this.closable&&`${t}-alert--closable`,this.showIcon&&`${t}-alert--show-icon`,!this.title&&this.closable&&`${t}-alert--right-adjust`,this.rtlEnabled&&`${t}-alert--rtl`],style:this.cssVars,role:`alert`};return this.visible?(b(),w(`div`,T({key:1},T(this.$attrs,r)),[m(()=>this.closable&&(b(),E(O,{clsPrefix:t,class:o(`${t}-alert__close`),onClick:this.handleCloseClick},null,8,[`clsPrefix`,`class`,`onClick`]))),m(()=>this.bordered&&(b(),w(`div`,{class:o(`${t}-alert__border`)},null,2))),m(()=>this.showIcon&&(b(),w(`div`,{class:o(`${t}-alert__icon`),"aria-hidden":`true`},[m(()=>l(n.icon,()=>[(b(),E(v,{clsPrefix:t},{default:()=>{switch(this.type){case`success`:return b(),E(A,{key:3});case`info`:return b(),E(j,{key:4});case`warning`:return b(),E(M,{key:5});case`error`:return b(),E(k,{key:6});default:return null}}},1032,[`clsPrefix`]))]))],2))),C(`div`,{class:o([`${t}-alert-body`,this.mergedBordered&&`${t}-alert-body--bordered`])},[m(()=>e(n.header,e=>{let n=e||this.title;return n?(b(),w(`div`,{key:2,class:o(`${t}-alert-body__title`)},[m(()=>n)],2)):null})),m(()=>n.default&&(b(),w(`div`,{class:o(`${t}-alert-body__content`)},[m(()=>n.default())],2)))],2)],16)):null}},1032,[`onAfterLeave`])}});export{R as t};