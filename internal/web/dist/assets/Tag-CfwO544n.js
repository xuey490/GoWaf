import{D as e,a as t,at as n,c as r,ct as i,d as a,g as o,it as s,l as c,lt as l,st as u,u as d,ut as f,w as p,y as m}from"./resolve-slot-D0ESJhJD.js";import{n as h,t as g}from"./use-rtl-B41EZXdW.js";import{a as _}from"./replaceable-CG1Qs4lR.js";import{H as v,S as y,St as b,V as x,Wt as S,_t as C,d as w,f as T,h as E,p as D}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{t as O}from"./_common-BWSN0c8x.js";import{t as k}from"./color-to-class-G4jmrDIg.js";import{t as A}from"./Close-swctlKIK.js";function j(e){let{textColor2:t,primaryColorHover:n,primaryColorPressed:r,primaryColor:i,infoColor:o,successColor:s,warningColor:c,errorColor:l,baseColor:u,borderColor:d,opacityDisabled:f,tagColor:p,closeIconColor:m,closeIconColorHover:h,closeIconColorPressed:g,borderRadiusSmall:_,fontSizeMini:v,fontSizeTiny:y,fontSizeSmall:b,fontSizeMedium:x,heightMini:S,heightTiny:C,heightSmall:w,heightMedium:T,closeColorHover:E,closeColorPressed:D,buttonColor2Hover:k,buttonColor2Pressed:A,fontWeightStrong:j}=e;return{...O,closeBorderRadius:_,heightTiny:S,heightSmall:C,heightMedium:w,heightLarge:T,borderRadius:_,opacityDisabled:f,fontSizeTiny:v,fontSizeSmall:y,fontSizeMedium:b,fontSizeLarge:x,fontWeightStrong:j,textColorCheckable:t,textColorHoverCheckable:t,textColorPressedCheckable:t,textColorChecked:u,colorCheckable:`#0000`,colorHoverCheckable:k,colorPressedCheckable:A,colorChecked:i,colorCheckedHover:n,colorCheckedPressed:r,border:`1px solid ${d}`,textColor:t,color:p,colorBordered:`rgb(250, 250, 252)`,closeIconColor:m,closeIconColorHover:h,closeIconColorPressed:g,closeColorHover:E,closeColorPressed:D,borderPrimary:`1px solid ${a(i,{alpha:.3})}`,textColorPrimary:i,colorPrimary:a(i,{alpha:.12}),colorBorderedPrimary:a(i,{alpha:.1}),closeIconColorPrimary:i,closeIconColorHoverPrimary:i,closeIconColorPressedPrimary:i,closeColorHoverPrimary:a(i,{alpha:.12}),closeColorPressedPrimary:a(i,{alpha:.18}),borderInfo:`1px solid ${a(o,{alpha:.3})}`,textColorInfo:o,colorInfo:a(o,{alpha:.12}),colorBorderedInfo:a(o,{alpha:.1}),closeIconColorInfo:o,closeIconColorHoverInfo:o,closeIconColorPressedInfo:o,closeColorHoverInfo:a(o,{alpha:.12}),closeColorPressedInfo:a(o,{alpha:.18}),borderSuccess:`1px solid ${a(s,{alpha:.3})}`,textColorSuccess:s,colorSuccess:a(s,{alpha:.12}),colorBorderedSuccess:a(s,{alpha:.1}),closeIconColorSuccess:s,closeIconColorHoverSuccess:s,closeIconColorPressedSuccess:s,closeColorHoverSuccess:a(s,{alpha:.12}),closeColorPressedSuccess:a(s,{alpha:.18}),borderWarning:`1px solid ${a(c,{alpha:.35})}`,textColorWarning:c,colorWarning:a(c,{alpha:.15}),colorBorderedWarning:a(c,{alpha:.12}),closeIconColorWarning:c,closeIconColorHoverWarning:c,closeIconColorPressedWarning:c,closeColorHoverWarning:a(c,{alpha:.12}),closeColorPressedWarning:a(c,{alpha:.18}),borderError:`1px solid ${a(l,{alpha:.23})}`,textColorError:l,colorError:a(l,{alpha:.1}),colorBorderedError:a(l,{alpha:.08}),closeIconColorError:l,closeIconColorHoverError:l,closeIconColorPressedError:l,closeColorHoverError:a(l,{alpha:.12}),closeColorPressedError:a(l,{alpha:.18})}}var M={name:`Tag`,common:d,self:j},N={color:Object,type:{type:String,default:`default`},round:Boolean,size:String,closable:Boolean,disabled:{type:Boolean,default:void 0}},P=n(`tag`,`
 --n-close-margin: var(--n-close-margin-top) var(--n-close-margin-right) var(--n-close-margin-bottom) var(--n-close-margin-left);
 white-space: nowrap;
 position: relative;
 box-sizing: border-box;
 cursor: default;
 display: inline-flex;
 align-items: center;
 flex-wrap: nowrap;
 padding: var(--n-padding);
 border-radius: var(--n-border-radius);
 color: var(--n-text-color);
 background-color: var(--n-color);
 transition: 
 border-color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier),
 opacity .3s var(--n-bezier);
 line-height: 1;
 height: var(--n-height);
 font-size: var(--n-font-size);
`,[i(`strong`,`
 font-weight: var(--n-font-weight-strong);
 `),u(`border`,`
 pointer-events: none;
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 border-radius: inherit;
 border: var(--n-border);
 transition: border-color .3s var(--n-bezier);
 `),u(`icon`,`
 display: flex;
 margin: 0 4px 0 0;
 color: var(--n-text-color);
 transition: color .3s var(--n-bezier);
 font-size: var(--n-avatar-size-override);
 `),u(`avatar`,`
 display: flex;
 margin: 0 6px 0 0;
 `),u(`close`,`
 margin: var(--n-close-margin);
 transition:
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
 `),i(`round`,`
 padding: 0 calc(var(--n-height) / 3);
 border-radius: calc(var(--n-height) / 2);
 `,[u(`icon`,`
 margin: 0 4px 0 calc((var(--n-height) - 8px) / -2);
 `),u(`avatar`,`
 margin: 0 6px 0 calc((var(--n-height) - 8px) / -2);
 `),i(`closable`,`
 padding: 0 calc(var(--n-height) / 4) 0 calc(var(--n-height) / 3);
 `)]),i(`icon, avatar`,[i(`round`,`
 padding: 0 calc(var(--n-height) / 3) 0 calc(var(--n-height) / 2);
 `)]),i(`disabled`,`
 cursor: not-allowed !important;
 opacity: var(--n-opacity-disabled);
 `),i(`checkable`,`
 cursor: pointer;
 box-shadow: none;
 color: var(--n-text-color-checkable);
 background-color: var(--n-color-checkable);
 `,[l(`disabled`,[s(`&:hover`,`background-color: var(--n-color-hover-checkable);`,[l(`checked`,`color: var(--n-text-color-hover-checkable);`)]),s(`&:active`,`background-color: var(--n-color-pressed-checkable);`,[l(`checked`,`color: var(--n-text-color-pressed-checkable);`)])]),i(`checked`,`
 color: var(--n-text-color-checked);
 background-color: var(--n-color-checked);
 `,[l(`disabled`,[s(`&:hover`,`background-color: var(--n-color-checked-hover);`),s(`&:active`,`background-color: var(--n-color-checked-pressed);`)])])])]),F=[`onClick`,`onMouseenter`,`onMouseleave`],I={...r.props,...N,bordered:{type:Boolean,default:void 0},checked:Boolean,checkable:Boolean,strong:Boolean,triggerClickOnClose:Boolean,onClose:[Array,Function],onMouseenter:Function,onMouseleave:Function,"onUpdate:checked":Function,onUpdateChecked:Function,internalCloseFocusable:{type:Boolean,default:!0},internalCloseIsButtonTag:{type:Boolean,default:!0},onCheckedChange:Function},L=e(`n-tag`),R=y({name:`Tag`,props:I,slots:Object,setup(e){let t=C(null),{mergedBorderedRef:n,mergedClsPrefixRef:i,inlineThemeDisabled:a,mergedRtlRef:o,mergedComponentPropsRef:s}=p(e),l=w(()=>e.size||s?.value?.Tag?.size||`medium`),u=r(`Tag`,`-tag`,P,M,e,i);v(L,{roundRef:b(e,`round`)});function d(){if(!e.disabled&&e.checkable){let{checked:t,onCheckedChange:n,onUpdateChecked:r,"onUpdate:checked":i}=e;r&&r(!t),i&&i(!t),n&&n(!t)}}function m(t){if(e.triggerClickOnClose||t.stopPropagation(),!e.disabled){let{onClose:n}=e;n&&h(n,t)}}let y={setTextContent(e){let{value:n}=t;n&&(n.textContent=e)}},x=g(`Tag`,o,i),S=w(()=>{let{type:t,color:{color:r,textColor:i}={}}=e,a=l.value,{common:{cubicBezierEaseInOut:o},self:{padding:s,closeMargin:c,borderRadius:d,opacityDisabled:p,textColorCheckable:m,textColorHoverCheckable:h,textColorPressedCheckable:g,textColorChecked:v,colorCheckable:y,colorHoverCheckable:b,colorPressedCheckable:x,colorChecked:S,colorCheckedHover:C,colorCheckedPressed:w,closeBorderRadius:T,fontWeightStrong:E,[f(`colorBordered`,t)]:D,[f(`closeSize`,a)]:O,[f(`closeIconSize`,a)]:k,[f(`fontSize`,a)]:A,[f(`height`,a)]:j,[f(`color`,t)]:M,[f(`textColor`,t)]:N,[f(`border`,t)]:P,[f(`closeIconColor`,t)]:F,[f(`closeIconColorHover`,t)]:I,[f(`closeIconColorPressed`,t)]:L,[f(`closeColorHover`,t)]:R,[f(`closeColorPressed`,t)]:z}}=u.value,B=_(c);return{"--n-font-weight-strong":E,"--n-avatar-size-override":`calc(${j} - 8px)`,"--n-bezier":o,"--n-border-radius":d,"--n-border":P,"--n-close-icon-size":k,"--n-close-color-pressed":z,"--n-close-color-hover":R,"--n-close-border-radius":T,"--n-close-icon-color":F,"--n-close-icon-color-hover":I,"--n-close-icon-color-pressed":L,"--n-close-icon-color-disabled":F,"--n-close-margin-top":B.top,"--n-close-margin-right":B.right,"--n-close-margin-bottom":B.bottom,"--n-close-margin-left":B.left,"--n-close-size":O,"--n-color":r||(n.value?D:M),"--n-color-checkable":y,"--n-color-checked":S,"--n-color-checked-hover":C,"--n-color-checked-pressed":w,"--n-color-hover-checkable":b,"--n-color-pressed-checkable":x,"--n-font-size":A,"--n-height":j,"--n-opacity-disabled":p,"--n-padding":s,"--n-text-color":i||N,"--n-text-color-checkable":m,"--n-text-color-checked":v,"--n-text-color-hover-checkable":h,"--n-text-color-pressed-checkable":g}}),T=a?c(`tag`,w(()=>{let t=``,{type:r,color:{color:i,textColor:a}={}}=e;return t+=r[0],t+=l.value[0],i&&(t+=`a${k(i)}`),a&&(t+=`b${k(a)}`),n.value&&(t+=`c`),t}),S,e):void 0;return{...y,rtlEnabled:x,mergedClsPrefix:i,contentRef:t,mergedBordered:n,handleClick:d,handleCloseClick:m,cssVars:a?void 0:S,themeClass:T?.themeClass,onRender:T?.onRender}},render(){let{mergedClsPrefix:e,rtlEnabled:n,closable:r,color:{borderColor:i}={},round:a,onRender:s,$slots:c}=this;s?.();let l=t(c.avatar,t=>t&&(x(),E(`div`,{class:o(`${e}-tag__avatar`)},[m(()=>t)],2))),u=t(c.icon,t=>t&&(x(),E(`div`,{class:o(`${e}-tag__icon`)},[m(()=>t)],2)));return x(),E(`div`,{class:o([`${e}-tag`,this.themeClass,{[`${e}-tag--rtl`]:n,[`${e}-tag--strong`]:this.strong,[`${e}-tag--disabled`]:this.disabled,[`${e}-tag--checkable`]:this.checkable,[`${e}-tag--checked`]:this.checkable&&this.checked,[`${e}-tag--round`]:a,[`${e}-tag--avatar`]:l,[`${e}-tag--icon`]:u,[`${e}-tag--closable`]:r}]),style:S(this.cssVars),onClick:this.handleClick,onMouseenter:this.onMouseenter,onMouseleave:this.onMouseleave},[m(()=>u||l),T(`span`,{class:o(`${e}-tag__content`),ref:`contentRef`},[m(()=>this.$slots.default?.())],2),!this.checkable&&r?(x(),D(A,{key:0,clsPrefix:e,class:o(`${e}-tag__close`),disabled:this.disabled,onClick:this.handleCloseClick,focusable:this.internalCloseFocusable,round:a,isButtonTag:this.internalCloseIsButtonTag,absolute:!0},null,8,[`clsPrefix`,`class`,`disabled`,`onClick`,`focusable`,`round`,`isButtonTag`])):m(()=>null),!this.checkable&&this.mergedBordered?(x(),E(`div`,{key:2,class:o(`${e}-tag__border`),style:S({borderColor:i})},null,6)):m(()=>null)],46,F)}});export{L as n,R as t};