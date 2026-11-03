import{S as e,at as t,c as n,ct as r,g as i,it as a,k as o,l as s,st as c,v as l,w as u,y as d}from"./resolve-slot-D0ESJhJD.js";import{n as f,t as p}from"./use-rtl-B41EZXdW.js";import{M as m,N as h,O as g,_,g as v,k as y,l as b,v as x,y as S}from"./light-D4diJaDS.js";import{pt as C}from"./user-ZPntuAC8.js";import{a as w,t as T}from"./Scrollbar-D9NqNphl.js";import{E,F as D,H as O,O as k,S as A,St as j,V as M,Wt as N,_t as P,d as F,f as I,h as L,i as R,it as z,j as B,ot as V,p as H,rt as U}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{a as W,t as G}from"./runtime-dom.esm-bundler-B3Pl3ttw.js";import{a as ee}from"./icon-switch.cssr-o4i5UUsg.js";import{r as K,v as q}from"./Modal-BgtV8YUs.js";import{t as J}from"./use-merged-state-KQOIh2IK.js";import{t as Y}from"./format-length-BwSo_EUP.js";import{t as X}from"./Close-swctlKIK.js";var Z=[`onMouseenter`,`onMouseleave`,`onMousedown`],Q={key:1,role:`none`},te=A({name:`NDrawerContent`,inheritAttrs:!1,props:{blockScroll:Boolean,show:{type:Boolean,default:void 0},displayDirective:{type:String,required:!0},placement:{type:String,required:!0},contentClass:String,contentStyle:[Object,String],nativeScrollbar:{type:Boolean,required:!0},scrollbarProps:Object,trapFocus:{type:Boolean,default:!0},autoFocus:{type:Boolean,default:!0},showMask:{type:[Boolean,String],required:!0},maxWidth:Number,maxHeight:Number,minWidth:Number,minHeight:Number,resizable:Boolean,onClickoutside:Function,onAfterLeave:Function,onAfterEnter:Function,onEsc:Function},setup(e){let t=P(!!e.show),n=P(null),r=k(h),i=0,a=``,o=null,s=P(!1),c=P(!1),l=F(()=>e.placement===`top`||e.placement===`bottom`),{mergedClsPrefixRef:d,mergedRtlRef:f}=u(e),_=p(`Drawer`,f,d),v=M,b=e=>{c.value=!0,i=l.value?e.clientY:e.clientX,a=document.body.style.cursor,document.body.style.cursor=l.value?`ns-resize`:`ew-resize`,document.body.addEventListener(`mousemove`,j),document.body.addEventListener(`mouseleave`,v),document.body.addEventListener(`mouseup`,M)},x=()=>{o!==null&&(window.clearTimeout(o),o=null),c.value?s.value=!0:o=window.setTimeout(()=>{s.value=!0},300)},C=()=>{o!==null&&(window.clearTimeout(o),o=null),s.value=!1},{doUpdateHeight:w,doUpdateWidth:T}=r,E=t=>{let{maxWidth:n}=e;if(n&&t>n)return n;let{minWidth:r}=e;return r&&t<r?r:t},A=t=>{let{maxHeight:n}=e;if(n&&t>n)return n;let{minHeight:r}=e;return r&&t<r?r:t};function j(t){if(c.value){if(l.value){let r=n.value?.offsetHeight||0,a=i-t.clientY;r+=e.placement===`bottom`?a:-a,r=A(r),w(r),i=t.clientY}else{let r=n.value?.offsetWidth||0,a=i-t.clientX;r+=e.placement===`right`?a:-a,r=E(r),T(r),i=t.clientX}}}function M(){c.value&&(i=0,c.value=!1,document.body.style.cursor=a,document.body.removeEventListener(`mousemove`,j),document.body.removeEventListener(`mouseup`,M),document.body.removeEventListener(`mouseleave`,v))}z(()=>{e.show&&(t.value=!0)}),U(()=>e.show,e=>{e||M()}),D(()=>{M()});let N=F(()=>{let{show:t}=e,n=[[W,t]];return e.showMask||n.push([S,e.onClickoutside,void 0,{capture:!0}]),n});function I(){t.value=!1,e.onAfterLeave?.()}return q(F(()=>e.blockScroll&&t.value)),O(m,n),O(g,null),O(y,null),{bodyRef:n,rtlEnabled:_,mergedClsPrefix:r.mergedClsPrefixRef,isMounted:r.isMountedRef,mergedTheme:r.mergedThemeRef,displayed:t,transitionName:F(()=>({right:`slide-in-from-right-transition`,left:`slide-in-from-left-transition`,top:`slide-in-from-top-transition`,bottom:`slide-in-from-bottom-transition`})[e.placement]),handleAfterLeave:I,bodyDirectives:N,handleMousedownResizeTrigger:b,handleMouseenterResizeTrigger:x,handleMouseleaveResizeTrigger:C,isDragging:c,isHoverOnResizeTrigger:s}},render(){let{$slots:e,mergedClsPrefix:t}=this;return this.displayDirective===`show`||this.displayed||this.show?V((M(),L(`div`,Q,[(M(),H(v,{disabled:!this.showMask||!this.trapFocus,active:this.show,autoFocus:this.autoFocus,onEsc:this.onEsc},{default:()=>(M(),H(G,{name:this.transitionName,appear:this.isMounted,onAfterEnter:this.onAfterEnter,onAfterLeave:this.handleAfterLeave},{default:()=>V(E(`div`,B(this.$attrs,{role:`dialog`,ref:`bodyRef`,"aria-modal":`true`,class:[`${t}-drawer`,this.rtlEnabled&&`${t}-drawer--rtl`,`${t}-drawer--${this.placement}-placement`,this.isDragging&&`${t}-drawer--unselectable`,this.nativeScrollbar&&`${t}-drawer--native-scrollbar`]}),[this.resizable?(M(),L(`div`,{key:2,class:i([`${t}-drawer__resize-trigger`,(this.isDragging||this.isHoverOnResizeTrigger)&&`${t}-drawer__resize-trigger--hover`]),onMouseenter:this.handleMouseenterResizeTrigger,onMouseleave:this.handleMouseleaveResizeTrigger,onMousedown:this.handleMousedownResizeTrigger},null,42,Z)):null,this.nativeScrollbar?(M(),L(`div`,{key:3,class:i([`${t}-drawer-content-wrapper`,this.contentClass]),style:N(this.contentStyle),role:`none`},[d(()=>e.default?.())],6)):(M(),H(T,B({key:4},this.scrollbarProps,{contentStyle:this.contentStyle,contentClass:[`${t}-drawer-content-wrapper`,this.contentClass],theme:this.mergedTheme.peers.Scrollbar,themeOverrides:this.mergedTheme.peerOverrides.Scrollbar}),l(e),1040,[`contentStyle`,`contentClass`,`theme`,`themeOverrides`]))]),this.bodyDirectives)},1032,[`name`,`appear`,`onAfterEnter`,`onAfterLeave`]))},1032,[`disabled`,`active`,`autoFocus`,`onEsc`]))])),[[W,this.displayDirective===`if`||this.displayed||this.show]]):null}}),{cubicBezierEaseIn:ne,cubicBezierEaseOut:re}=e;function ie({duration:e=`0.3s`,leaveDuration:t=`0.2s`,name:n=`slide-in-from-bottom`}={}){return[a(`&.${n}-transition-leave-active`,{transition:`transform ${t} ${ne}`}),a(`&.${n}-transition-enter-active`,{transition:`transform ${e} ${re}`}),a(`&.${n}-transition-enter-to`,{transform:`translateY(0)`}),a(`&.${n}-transition-enter-from`,{transform:`translateY(100%)`}),a(`&.${n}-transition-leave-from`,{transform:`translateY(0)`}),a(`&.${n}-transition-leave-to`,{transform:`translateY(100%)`})]}var{cubicBezierEaseIn:ae,cubicBezierEaseOut:oe}=e;function se({duration:e=`0.3s`,leaveDuration:t=`0.2s`,name:n=`slide-in-from-left`}={}){return[a(`&.${n}-transition-leave-active`,{transition:`transform ${t} ${ae}`}),a(`&.${n}-transition-enter-active`,{transition:`transform ${e} ${oe}`}),a(`&.${n}-transition-enter-to`,{transform:`translateX(0)`}),a(`&.${n}-transition-enter-from`,{transform:`translateX(-100%)`}),a(`&.${n}-transition-leave-from`,{transform:`translateX(0)`}),a(`&.${n}-transition-leave-to`,{transform:`translateX(-100%)`})]}var{cubicBezierEaseIn:ce,cubicBezierEaseOut:le}=e;function $({duration:e=`0.3s`,leaveDuration:t=`0.2s`,name:n=`slide-in-from-right`}={}){return[a(`&.${n}-transition-leave-active`,{transition:`transform ${t} ${ce}`}),a(`&.${n}-transition-enter-active`,{transition:`transform ${e} ${le}`}),a(`&.${n}-transition-enter-to`,{transform:`translateX(0)`}),a(`&.${n}-transition-enter-from`,{transform:`translateX(100%)`}),a(`&.${n}-transition-leave-from`,{transform:`translateX(0)`}),a(`&.${n}-transition-leave-to`,{transform:`translateX(100%)`})]}var{cubicBezierEaseIn:ue,cubicBezierEaseOut:de}=e;function fe({duration:e=`0.3s`,leaveDuration:t=`0.2s`,name:n=`slide-in-from-top`}={}){return[a(`&.${n}-transition-leave-active`,{transition:`transform ${t} ${ue}`}),a(`&.${n}-transition-enter-active`,{transition:`transform ${e} ${de}`}),a(`&.${n}-transition-enter-to`,{transform:`translateY(0)`}),a(`&.${n}-transition-enter-from`,{transform:`translateY(-100%)`}),a(`&.${n}-transition-leave-from`,{transform:`translateY(0)`}),a(`&.${n}-transition-leave-to`,{transform:`translateY(-100%)`})]}var pe=a([t(`drawer`,`
 word-break: break-word;
 line-height: var(--n-line-height);
 position: absolute;
 pointer-events: all;
 box-shadow: var(--n-box-shadow);
 transition:
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
 background-color: var(--n-color);
 color: var(--n-text-color);
 box-sizing: border-box;
 `,[$(),se(),fe(),ie(),r(`unselectable`,`
 user-select: none; 
 -webkit-user-select: none;
 `),r(`native-scrollbar`,[t(`drawer-content-wrapper`,`
 overflow: auto;
 height: 100%;
 `)]),c(`resize-trigger`,`
 position: absolute;
 background-color: #0000;
 transition: background-color .3s var(--n-bezier);
 `,[r(`hover`,`
 background-color: var(--n-resize-trigger-color-hover);
 `)]),t(`drawer-content-wrapper`,`
 box-sizing: border-box;
 `),t(`drawer-content`,`
 height: 100%;
 display: flex;
 flex-direction: column;
 `,[r(`native-scrollbar`,[t(`drawer-body-content-wrapper`,`
 height: 100%;
 overflow: auto;
 `)]),t(`drawer-body`,`
 flex: 1 0 0;
 overflow: hidden;
 `),t(`drawer-body-content-wrapper`,`
 box-sizing: border-box;
 padding: var(--n-body-padding);
 `),t(`drawer-header`,`
 font-weight: var(--n-title-font-weight);
 line-height: 1;
 font-size: var(--n-title-font-size);
 color: var(--n-title-text-color);
 padding: var(--n-header-padding);
 transition: border .3s var(--n-bezier);
 border-bottom: 1px solid var(--n-divider-color);
 border-bottom: var(--n-header-border-bottom);
 display: flex;
 justify-content: space-between;
 align-items: center;
 `,[c(`main`,`
 flex: 1;
 `),c(`close`,`
 margin-left: 6px;
 transition:
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
 `)]),t(`drawer-footer`,`
 display: flex;
 justify-content: flex-end;
 border-top: var(--n-footer-border-top);
 transition: border .3s var(--n-bezier);
 padding: var(--n-footer-padding);
 `)]),r(`right-placement`,`
 top: 0;
 bottom: 0;
 right: 0;
 border-top-left-radius: var(--n-border-radius);
 border-bottom-left-radius: var(--n-border-radius);
 `,[c(`resize-trigger`,`
 width: 3px;
 height: 100%;
 top: 0;
 left: 0;
 transform: translateX(-1.5px);
 cursor: ew-resize;
 `)]),r(`left-placement`,`
 top: 0;
 bottom: 0;
 left: 0;
 border-top-right-radius: var(--n-border-radius);
 border-bottom-right-radius: var(--n-border-radius);
 `,[c(`resize-trigger`,`
 width: 3px;
 height: 100%;
 top: 0;
 right: 0;
 transform: translateX(1.5px);
 cursor: ew-resize;
 `)]),r(`top-placement`,`
 top: 0;
 left: 0;
 right: 0;
 border-bottom-left-radius: var(--n-border-radius);
 border-bottom-right-radius: var(--n-border-radius);
 `,[c(`resize-trigger`,`
 width: 100%;
 height: 3px;
 bottom: 0;
 left: 0;
 transform: translateY(1.5px);
 cursor: ns-resize;
 `)]),r(`bottom-placement`,`
 left: 0;
 bottom: 0;
 right: 0;
 border-top-left-radius: var(--n-border-radius);
 border-top-right-radius: var(--n-border-radius);
 `,[c(`resize-trigger`,`
 width: 100%;
 height: 3px;
 top: 0;
 left: 0;
 transform: translateY(-1.5px);
 cursor: ns-resize;
 `)])]),a(`body`,[a(`>`,[t(`drawer-container`,`
 position: fixed;
 `)])]),t(`drawer-container`,`
 position: relative;
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 pointer-events: none;
 `,[a(`> *`,`
 pointer-events: all;
 `)]),t(`drawer-mask`,`
 background-color: rgba(0, 0, 0, .3);
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 `,[r(`invisible`,`
 background-color: rgba(0, 0, 0, 0)
 `),w({enterDuration:`0.2s`,leaveDuration:`0.2s`,enterCubicBezier:`var(--n-bezier-in)`,leaveCubicBezier:`var(--n-bezier-out)`})])]),me=[`onClick`],he={...n.props,show:Boolean,width:[Number,String],height:[Number,String],placement:{type:String,default:`right`},maskClosable:{type:Boolean,default:!0},showMask:{type:[Boolean,String],default:!0},to:[String,Object],displayDirective:{type:String,default:`if`},nativeScrollbar:{type:Boolean,default:!0},zIndex:Number,onMaskClick:Function,scrollbarProps:Object,contentClass:String,contentStyle:[Object,String],trapFocus:{type:Boolean,default:!0},onEsc:Function,autoFocus:{type:Boolean,default:!0},closeOnEsc:{type:Boolean,default:!0},blockScroll:{type:Boolean,default:!0},maxWidth:Number,maxHeight:Number,minWidth:Number,minHeight:Number,resizable:Boolean,defaultWidth:{type:[Number,String],default:251},defaultHeight:{type:[Number,String],default:251},onUpdateWidth:[Function,Array],onUpdateHeight:[Function,Array],"onUpdate:width":[Function,Array],"onUpdate:height":[Function,Array],"onUpdate:show":[Function,Array],onUpdateShow:[Function,Array],onAfterEnter:Function,onAfterLeave:Function,drawerStyle:[String,Object],drawerClass:String,target:null,onShow:Function,onHide:Function},ge=A({name:`Drawer`,inheritAttrs:!1,props:he,setup(e){let{mergedClsPrefixRef:t,namespaceRef:r,inlineThemeDisabled:i}=u(e),a=ee(),o=n(`Drawer`,`-drawer`,pe,C,e,t),c=P(e.defaultWidth),l=P(e.defaultHeight),d=J(j(e,`width`),c),p=J(j(e,`height`),l),m=F(()=>{let{placement:t}=e;return t===`top`||t===`bottom`?``:Y(d.value)}),g=F(()=>{let{placement:t}=e;return t===`left`||t===`right`?``:Y(p.value)}),_=t=>{let{onUpdateWidth:n,"onUpdate:width":r}=e;n&&f(n,t),r&&f(r,t),c.value=t},v=t=>{let{onUpdateHeight:n,"onUpdate:width":r}=e;n&&f(n,t),r&&f(r,t),l.value=t},y=F(()=>[{width:m.value,height:g.value},e.drawerStyle||``]);function x(t){let{onMaskClick:n,maskClosable:r}=e;r&&E(!1),n&&n(t)}function S(e){x(e)}let w=K();function T(t){e.onEsc?.(),e.show&&e.closeOnEsc&&b(t)&&(w.value||E(!1))}function E(t){let{onHide:n,onUpdateShow:r,"onUpdate:show":i}=e;r&&f(r,t),i&&f(i,t),n&&!t&&f(n,t)}O(h,{isMountedRef:a,mergedThemeRef:o,mergedClsPrefixRef:t,doUpdateShow:E,doUpdateHeight:v,doUpdateWidth:_});let D=F(()=>{let{common:{cubicBezierEaseInOut:e,cubicBezierEaseIn:t,cubicBezierEaseOut:n},self:{color:r,textColor:i,boxShadow:a,lineHeight:s,headerPadding:c,footerPadding:l,borderRadius:u,bodyPadding:d,titleFontSize:f,titleTextColor:p,titleFontWeight:m,headerBorderBottom:h,footerBorderTop:g,closeIconColor:_,closeIconColorHover:v,closeIconColorPressed:y,closeColorHover:b,closeColorPressed:x,closeIconSize:S,closeSize:C,closeBorderRadius:w,resizableTriggerColorHover:T}}=o.value;return{"--n-line-height":s,"--n-color":r,"--n-border-radius":u,"--n-text-color":i,"--n-box-shadow":a,"--n-bezier":e,"--n-bezier-out":n,"--n-bezier-in":t,"--n-header-padding":c,"--n-body-padding":d,"--n-footer-padding":l,"--n-title-text-color":p,"--n-title-font-size":f,"--n-title-font-weight":m,"--n-header-border-bottom":h,"--n-footer-border-top":g,"--n-close-icon-color":_,"--n-close-icon-color-hover":v,"--n-close-icon-color-pressed":y,"--n-close-size":C,"--n-close-color-hover":b,"--n-close-color-pressed":x,"--n-close-icon-size":S,"--n-close-border-radius":w,"--n-resize-trigger-color-hover":T}}),k=i?s(`drawer`,void 0,D,e):void 0;return{mergedClsPrefix:t,namespace:r,mergedBodyStyle:y,handleOutsideClick:S,handleMaskClick:x,handleEsc:T,mergedTheme:o,cssVars:i?void 0:D,themeClass:k?.themeClass,onRender:k?.onRender,isMounted:a}},render(){let{mergedClsPrefix:e}=this;return M(),H(_,{to:this.to,show:this.show},{default:()=>(this.onRender?.(),V((M(),L(`div`,{class:i([`${e}-drawer-container`,this.namespace,this.themeClass]),style:N(this.cssVars),role:`none`},[this.showMask?(M(),H(G,{key:0,name:`fade-in-transition`,appear:this.isMounted},{default:()=>this.show?(M(),L(`div`,{key:1,"aria-hidden":!0,class:i([`${e}-drawer-mask`,this.showMask===`transparent`&&`${e}-drawer-mask--invisible`]),onClick:this.handleMaskClick},null,10,me)):null},1032,[`appear`])):d(()=>null),(M(),H(te,B(this.$attrs,{class:[this.drawerClass,this.$attrs.class],style:[this.mergedBodyStyle,this.$attrs.style],blockScroll:this.blockScroll,contentStyle:this.contentStyle,contentClass:this.contentClass,placement:this.placement,scrollbarProps:this.scrollbarProps,show:this.show,displayDirective:this.displayDirective,nativeScrollbar:this.nativeScrollbar,onAfterEnter:this.onAfterEnter,onAfterLeave:this.onAfterLeave,trapFocus:this.trapFocus,autoFocus:this.autoFocus,resizable:this.resizable,maxHeight:this.maxHeight,minHeight:this.minHeight,maxWidth:this.maxWidth,minWidth:this.minWidth,showMask:this.showMask,onEsc:this.handleEsc,onClickoutside:this.handleOutsideClick}),l(this.$slots),1040,[`class`,`style`,`blockScroll`,`contentStyle`,`contentClass`,`placement`,`scrollbarProps`,`show`,`displayDirective`,`nativeScrollbar`,`onAfterEnter`,`onAfterLeave`,`trapFocus`,`autoFocus`,`resizable`,`maxHeight`,`minHeight`,`maxWidth`,`minWidth`,`showMask`,`onEsc`,`onClickoutside`]))],6)),[[x,{zIndex:this.zIndex,enabled:this.show}]]))},1032,[`to`,`show`])}}),_e=A({name:`DrawerContent`,props:{title:String,headerClass:String,headerStyle:[Object,String],footerClass:String,footerStyle:[Object,String],bodyClass:String,bodyStyle:[Object,String],bodyContentClass:String,bodyContentStyle:[Object,String],nativeScrollbar:{type:Boolean,default:!0},scrollbarProps:Object,closable:Boolean},slots:Object,setup(){let e=k(h,null);e||o(`drawer-content`,"`n-drawer-content` must be placed inside `n-drawer`.");let{doUpdateShow:t}=e;function n(){t(!1)}return{handleCloseClick:n,mergedTheme:e.mergedThemeRef,mergedClsPrefix:e.mergedClsPrefixRef}},render(){let{title:e,mergedClsPrefix:t,nativeScrollbar:n,mergedTheme:r,bodyClass:a,bodyStyle:o,bodyContentClass:s,bodyContentStyle:c,headerClass:u,headerStyle:f,footerClass:p,footerStyle:m,scrollbarProps:h,closable:g,$slots:_}=this;return M(),L(`div`,{role:`none`,class:i([`${t}-drawer-content`,n&&`${t}-drawer-content--native-scrollbar`])},[_.header||e||g?(M(),L(`div`,{key:0,class:i([`${t}-drawer-header`,u]),style:N(f),role:`none`},[I(`div`,{class:i(`${t}-drawer-header__main`),role:`heading`,"aria-level":`1`},[_.header===void 0?(M(),L(R,{key:1},[d(()=>e)],64)):(M(),L(R,{key:0},[d(()=>_.header())],64))],2),d(()=>g&&(M(),H(X,{onClick:this.handleCloseClick,clsPrefix:t,class:i(`${t}-drawer-header__close`),absolute:!0},null,8,[`onClick`,`clsPrefix`,`class`])))],6)):d(()=>null),n?(M(),L(`div`,{key:2,class:i([`${t}-drawer-body`,a]),style:N(o),role:`none`},[I(`div`,{class:i([`${t}-drawer-body-content-wrapper`,s]),style:N(c),role:`none`},[d(()=>_.default?.())],6)],6)):(M(),H(T,B({key:3,themeOverrides:r.peerOverrides.Scrollbar,theme:r.peers.Scrollbar},h,{class:`${t}-drawer-body`,contentClass:[`${t}-drawer-body-content-wrapper`,s],contentStyle:c}),l(_),1040,[`themeOverrides`,`theme`,`class`,`contentClass`,`contentStyle`])),_.footer?(M(),L(`div`,{key:4,class:i([`${t}-drawer-footer`,p]),style:N(m),role:`none`},[d(()=>_.footer())],6)):d(()=>null)],2)}});export{ge as n,_e as t};