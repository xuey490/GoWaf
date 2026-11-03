import{a as e,at as t,c as n,ct as r,dt as i,ft as a,g as o,it as s,l as c,rt as l,st as u,t as d,u as f,ut as p,w as m,y as h}from"./resolve-slot-D0ESJhJD.js";import{n as g,t as _}from"./use-rtl-B41EZXdW.js";import{a as v}from"./replaceable-CG1Qs4lR.js";import{t as y,u as b}from"./Scrollbar-D9NqNphl.js";import{S as x,V as S,Wt as C,at as w,d as T,f as E,h as D,p as O}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{t as k}from"./Close-swctlKIK.js";var A={paddingSmall:`12px 16px 12px`,paddingMedium:`19px 24px 20px`,paddingLarge:`23px 32px 24px`,paddingHuge:`27px 40px 28px`,titleFontSizeSmall:`16px`,titleFontSizeMedium:`18px`,titleFontSizeLarge:`18px`,titleFontSizeHuge:`18px`,closeIconSize:`18px`,closeSize:`22px`};function j(e){let{primaryColor:t,borderRadius:n,lineHeight:r,fontSize:i,cardColor:a,textColor2:o,textColor1:s,dividerColor:c,fontWeightStrong:l,closeIconColor:u,closeIconColorHover:d,closeIconColorPressed:f,closeColorHover:p,closeColorPressed:m,modalColor:h,boxShadow1:g,popoverColor:_,actionColor:v}=e;return{...A,lineHeight:r,color:a,colorModal:h,colorPopover:_,colorTarget:t,colorEmbedded:v,colorEmbeddedModal:v,colorEmbeddedPopover:v,textColor:o,titleTextColor:s,borderColor:c,actionColor:v,titleFontWeight:l,closeColorHover:p,closeColorPressed:m,closeBorderRadius:n,closeIconColor:u,closeIconColorHover:d,closeIconColorPressed:f,fontSizeSmall:i,fontSizeMedium:i,fontSizeLarge:i,fontSizeHuge:i,boxShadow:g,borderRadius:n}}var M={name:`Card`,common:f,self:j},N=t(`card-content`,`
 flex: 1;
 min-width: 0;
 box-sizing: border-box;
 padding: 0 var(--n-padding-left) var(--n-padding-bottom) var(--n-padding-left);
 font-size: var(--n-font-size);
`),P=s([t(`card`,`
 font-size: var(--n-font-size);
 line-height: var(--n-line-height);
 display: flex;
 flex-direction: column;
 width: 100%;
 box-sizing: border-box;
 position: relative;
 border-radius: var(--n-border-radius);
 background-color: var(--n-color);
 color: var(--n-text-color);
 word-break: break-word;
 transition: 
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `,[l({background:`var(--n-color-modal)`}),r(`hoverable`,[s(`&:hover`,`box-shadow: var(--n-box-shadow);`)]),r(`content-segmented`,[s(`>`,[t(`card-content`,`
 padding-top: var(--n-padding-bottom);
 `),u(`content-scrollbar`,[s(`>`,[t(`scrollbar-container`,[s(`>`,[t(`card-content`,`
 padding-top: var(--n-padding-bottom);
 `)])])])])])]),r(`content-soft-segmented`,[s(`>`,[t(`card-content`,`
 margin: 0 var(--n-padding-left);
 padding: var(--n-padding-bottom) 0;
 `),u(`content-scrollbar`,[s(`>`,[t(`scrollbar-container`,[s(`>`,[t(`card-content`,`
 margin: 0 var(--n-padding-left);
 padding: var(--n-padding-bottom) 0;
 `)])])])])])]),r(`footer-segmented`,[s(`>`,[u(`footer`,`
 padding-top: var(--n-padding-bottom);
 `)])]),r(`footer-soft-segmented`,[s(`>`,[u(`footer`,`
 padding: var(--n-padding-bottom) 0;
 margin: 0 var(--n-padding-left);
 `)])]),s(`>`,[t(`card-header`,`
 box-sizing: border-box;
 display: flex;
 align-items: center;
 font-size: var(--n-title-font-size);
 padding:
 var(--n-padding-top)
 var(--n-padding-left)
 var(--n-padding-bottom)
 var(--n-padding-left);
 `,[u(`main`,`
 font-weight: var(--n-title-font-weight);
 transition: color .3s var(--n-bezier);
 flex: 1;
 min-width: 0;
 color: var(--n-title-text-color);
 `),u(`extra`,`
 display: flex;
 align-items: center;
 font-size: var(--n-font-size);
 font-weight: 400;
 transition: color .3s var(--n-bezier);
 color: var(--n-text-color);
 `),u(`close`,`
 margin: 0 0 0 8px;
 transition:
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
 `)]),u(`action`,`
 box-sizing: border-box;
 transition:
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 background-clip: padding-box;
 background-color: var(--n-action-color);
 `),N,t(`card-content`,[s(`&:first-child`,`
 padding-top: var(--n-padding-bottom);
 `)]),u(`content-scrollbar`,`
 display: flex;
 flex-direction: column;
 `,[s(`>`,[t(`scrollbar-container`,[s(`>`,[N])])]),s(`&:first-child >`,[t(`scrollbar-container`,[s(`>`,[t(`card-content`,`
 padding-top: var(--n-padding-bottom);
 `)])])])]),u(`footer`,`
 box-sizing: border-box;
 padding: 0 var(--n-padding-left) var(--n-padding-bottom) var(--n-padding-left);
 font-size: var(--n-font-size);
 `,[s(`&:first-child`,`
 padding-top: var(--n-padding-bottom);
 `)]),u(`action`,`
 background-color: var(--n-action-color);
 padding: var(--n-padding-bottom) var(--n-padding-left);
 border-bottom-left-radius: var(--n-border-radius);
 border-bottom-right-radius: var(--n-border-radius);
 `)]),t(`card-cover`,`
 overflow: hidden;
 width: 100%;
 border-radius: var(--n-border-radius) var(--n-border-radius) 0 0;
 `,[s(`img`,`
 display: block;
 width: 100%;
 `)]),r(`bordered`,`
 border: 1px solid var(--n-border-color);
 `,[s(`&:target`,`border-color: var(--n-color-target);`)]),r(`action-segmented`,[s(`>`,[u(`action`,[s(`&:not(:first-child)`,`
 border-top: 1px solid var(--n-border-color);
 `)])])]),r(`content-segmented, content-soft-segmented`,[s(`>`,[t(`card-content`,`
 transition: border-color 0.3s var(--n-bezier);
 `,[s(`&:not(:first-child)`,`
 border-top: 1px solid var(--n-border-color);
 `)]),u(`content-scrollbar`,`
 transition: border-color 0.3s var(--n-bezier);
 `,[s(`&:not(:first-child)`,`
 border-top: 1px solid var(--n-border-color);
 `)])])]),r(`footer-segmented, footer-soft-segmented`,[s(`>`,[u(`footer`,`
 transition: border-color 0.3s var(--n-bezier);
 `,[s(`&:not(:first-child)`,`
 border-top: 1px solid var(--n-border-color);
 `)])])]),r(`embedded`,`
 background-color: var(--n-color-embedded);
 `)]),i(t(`card`,`
 background: var(--n-color-modal);
 `,[r(`embedded`,`
 background-color: var(--n-color-embedded-modal);
 `)])),a(t(`card`,`
 background: var(--n-color-popover);
 `,[r(`embedded`,`
 background-color: var(--n-color-embedded-popover);
 `)]))]),F={title:[String,Function],contentClass:String,contentStyle:[Object,String],contentScrollable:Boolean,headerClass:String,headerStyle:[Object,String],headerExtraClass:String,headerExtraStyle:[Object,String],footerClass:String,footerStyle:[Object,String],embedded:Boolean,segmented:{type:[Boolean,Object],default:!1},size:String,bordered:{type:Boolean,default:!0},closable:Boolean,hoverable:Boolean,role:String,onClose:[Function,Array],tag:{type:String,default:`div`},cover:Function,content:[String,Function],footer:Function,action:Function,headerExtra:Function,closeFocusable:Boolean},I=b(F),L={...n.props,...F},R=x({name:`Card`,props:L,slots:Object,setup(e){let t=()=>{let{onClose:t}=e;t&&g(t)},{inlineThemeDisabled:r,mergedClsPrefixRef:i,mergedRtlRef:a,mergedComponentPropsRef:o}=m(e),s=n(`Card`,`-card`,P,M,e,i),l=_(`Card`,a,i),u=T(()=>e.size||o?.value?.Card?.size||`medium`),d=T(()=>{let e=u.value,{self:{color:t,colorModal:n,colorTarget:r,textColor:i,titleTextColor:a,titleFontWeight:o,borderColor:c,actionColor:l,borderRadius:d,lineHeight:f,closeIconColor:m,closeIconColorHover:h,closeIconColorPressed:g,closeColorHover:_,closeColorPressed:y,closeBorderRadius:b,closeIconSize:x,closeSize:S,boxShadow:C,colorPopover:w,colorEmbedded:T,colorEmbeddedModal:E,colorEmbeddedPopover:D,[p(`padding`,e)]:O,[p(`fontSize`,e)]:k,[p(`titleFontSize`,e)]:A},common:{cubicBezierEaseInOut:j}}=s.value,{top:M,left:N,bottom:P}=v(O);return{"--n-bezier":j,"--n-border-radius":d,"--n-color":t,"--n-color-modal":n,"--n-color-popover":w,"--n-color-embedded":T,"--n-color-embedded-modal":E,"--n-color-embedded-popover":D,"--n-color-target":r,"--n-text-color":i,"--n-line-height":f,"--n-action-color":l,"--n-title-text-color":a,"--n-title-font-weight":o,"--n-close-icon-color":m,"--n-close-icon-color-hover":h,"--n-close-icon-color-pressed":g,"--n-close-color-hover":_,"--n-close-color-pressed":y,"--n-border-color":c,"--n-box-shadow":C,"--n-padding-top":M,"--n-padding-bottom":P,"--n-padding-left":N,"--n-font-size":k,"--n-title-font-size":A,"--n-close-size":S,"--n-close-icon-size":x,"--n-close-border-radius":b}}),f=r?c(`card`,T(()=>u.value[0]),d,e):void 0;return{rtlEnabled:l,mergedClsPrefix:i,mergedTheme:s,handleCloseClick:t,cssVars:r?void 0:d,themeClass:f?.themeClass,onRender:f?.onRender}},render(){let{segmented:t,bordered:n,hoverable:r,mergedClsPrefix:i,rtlEnabled:a,onRender:s,embedded:c,tag:l,$slots:u}=this;return s?.(),S(),O(l,{class:o([`${i}-card`,this.themeClass,c&&`${i}-card--embedded`,{[`${i}-card--rtl`]:a,[`${i}-card--content-scrollable`]:this.contentScrollable,[`${i}-card--content${typeof t!=`boolean`&&t.content===`soft`?`-soft`:``}-segmented`]:t===!0||t!==!1&&t.content,[`${i}-card--footer${typeof t!=`boolean`&&t.footer===`soft`?`-soft`:``}-segmented`]:t===!0||t!==!1&&t.footer,[`${i}-card--action-segmented`]:t===!0||t!==!1&&t.action,[`${i}-card--bordered`]:n,[`${i}-card--hoverable`]:r}]),style:C(this.cssVars),role:this.role},{default:w(()=>[h(()=>e(u.cover,e=>{let t=this.cover?d([this.cover()]):e;return t&&(S(),D(`div`,{class:o(`${i}-card-cover`),role:`none`},[h(()=>t)],2))})),h(()=>e(u.header,t=>{let{title:n}=this,r=n?d(typeof n==`function`?[n()]:[n]):t;return r||this.closable?(S(),D(`div`,{key:1,class:o([`${i}-card-header`,this.headerClass]),style:C(this.headerStyle),role:`heading`},[E(`div`,{class:o(`${i}-card-header__main`),role:`heading`},[h(()=>r)],2),h(()=>e(u[`header-extra`],e=>{let t=this.headerExtra?d([this.headerExtra()]):e;return t&&(S(),D(`div`,{class:o([`${i}-card-header__extra`,this.headerExtraClass]),style:C(this.headerExtraStyle)},[h(()=>t)],6))})),h(()=>this.closable&&(S(),O(k,{clsPrefix:i,class:o(`${i}-card-header__close`),onClick:this.handleCloseClick,focusable:this.closeFocusable,absolute:!0},null,8,[`clsPrefix`,`class`,`onClick`,`focusable`])))],6)):null})),h(()=>e(u.default,e=>{let{content:t}=this,n=t?d(typeof t==`function`?[t()]:[t]):e;return n?this.contentScrollable?(S(),O(y,{key:2,class:o(`${i}-card__content-scrollbar`),contentClass:[`${i}-card-content`,this.contentClass],contentStyle:this.contentStyle},{default:()=>n},1032,[`class`,`contentClass`,`contentStyle`])):(S(),D(`div`,{key:3,class:o([`${i}-card-content`,this.contentClass]),style:C(this.contentStyle),role:`none`},[h(()=>n)],6)):null})),h(()=>e(u.footer,e=>{let t=this.footer?d([this.footer()]):e;return t&&(S(),D(`div`,{class:o([`${i}-card__footer`,this.footerClass]),style:C(this.footerStyle),role:`none`},[h(()=>t)],6))})),h(()=>e(u.action,e=>{let t=this.action?d([this.action()]):e;return t&&(S(),D(`div`,{class:o(`${i}-card__action`),role:`none`},[h(()=>t)],2))}))]),_:2},1032,[`class`,`style`,`role`])}});export{j as a,M as i,I as n,F as r,R as t};