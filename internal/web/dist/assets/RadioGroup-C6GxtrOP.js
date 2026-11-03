import{D as e,a as t,at as n,c as r,ct as i,g as a,h as o,it as s,l as c,lt as l,st as u,ut as d,w as f,y as p}from"./resolve-slot-D0ESJhJD.js";import{n as m,t as h}from"./use-rtl-B41EZXdW.js";import{s as g}from"./light-D4diJaDS.js";import{H as _,O as v,S as y,St as b,V as x,Wt as S,_t as C,d as w,f as T,h as E,p as D}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{i as O,o as k}from"./icon-switch.cssr-o4i5UUsg.js";import{t as A}from"./use-merged-state-KQOIh2IK.js";import{t as j}from"./flatten-q_1Pdy7y.js";import{t as M}from"./get-slot-6kXJmSMP.js";var N=n(`radio`,`
 line-height: var(--n-label-line-height);
 outline: none;
 position: relative;
 user-select: none;
 -webkit-user-select: none;
 display: inline-flex;
 align-items: flex-start;
 flex-wrap: nowrap;
 font-size: var(--n-font-size);
 word-break: break-word;
`,[i(`checked`,[u(`dot`,`
 background-color: var(--n-color-active);
 `)]),u(`dot-wrapper`,`
 position: relative;
 flex-shrink: 0;
 flex-grow: 0;
 width: var(--n-radio-size);
 `),n(`radio-input`,`
 position: absolute;
 border: 0;
 width: 0;
 height: 0;
 opacity: 0;
 margin: 0;
 `),u(`dot`,`
 position: absolute;
 top: 50%;
 left: 0;
 transform: translateY(-50%);
 height: var(--n-radio-size);
 width: var(--n-radio-size);
 background: var(--n-color);
 box-shadow: var(--n-box-shadow);
 border-radius: 50%;
 transition:
 background-color .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier);
 `,[s(`&::before`,`
 content: "";
 opacity: 0;
 position: absolute;
 left: 4px;
 top: 4px;
 height: calc(100% - 8px);
 width: calc(100% - 8px);
 border-radius: 50%;
 transform: scale(.8);
 background: var(--n-dot-color-active);
 transition: 
 opacity .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 transform .3s var(--n-bezier);
 `),i(`checked`,{boxShadow:`var(--n-box-shadow-active)`},[s(`&::before`,`
 opacity: 1;
 transform: scale(1);
 `)])]),u(`label`,`
 color: var(--n-text-color);
 padding: var(--n-label-padding);
 font-weight: var(--n-label-font-weight);
 display: inline-block;
 transition: color .3s var(--n-bezier);
 `),l(`disabled`,`
 cursor: pointer;
 `,[s(`&:hover`,[u(`dot`,{boxShadow:`var(--n-box-shadow-hover)`})]),i(`focus`,[s(`&:not(:active)`,[u(`dot`,{boxShadow:`var(--n-box-shadow-focus)`})])])]),i(`disabled`,`
 cursor: not-allowed;
 `,[u(`dot`,{boxShadow:`var(--n-box-shadow-disabled)`,backgroundColor:`var(--n-color-disabled)`},[s(`&::before`,{backgroundColor:`var(--n-dot-color-disabled)`}),i(`checked`,`
 opacity: 1;
 `)]),u(`label`,{color:`var(--n-text-color-disabled)`}),n(`radio-input`,`
 cursor: not-allowed;
 `)])]),P={name:String,value:{type:[String,Number,Boolean],default:`on`},checked:{type:Boolean,default:void 0},defaultChecked:Boolean,disabled:{type:Boolean,default:void 0},label:String,size:String,onUpdateChecked:[Function,Array],"onUpdate:checked":[Function,Array],checkedValue:{type:Boolean,default:void 0}},F=e(`n-radio-group`);function I(e){let t=v(F,null),{mergedClsPrefixRef:n,mergedComponentPropsRef:r}=f(e),i=O(e,{mergedSize(n){let{size:i}=e;if(i!==void 0)return i;if(t){let{mergedSizeRef:{value:e}}=t;if(e!==void 0)return e}return n?n.mergedSize.value:r?.value?.Radio?.size||`medium`},mergedDisabled(n){return!!(e.disabled||t?.disabledRef.value||n?.disabled.value)}}),{mergedSizeRef:a,mergedDisabledRef:o}=i,s=C(null),c=C(null),l=C(e.defaultChecked),u=b(e,`checked`),d=A(u,l),p=k(()=>t?t.valueRef.value===e.value:d.value),h=k(()=>{let{name:n}=e;if(n!==void 0)return n;if(t)return t.nameRef.value}),g=C(!1);function _(){if(t){let{doUpdateValue:n}=t,{value:r}=e;m(n,r)}else{let{onUpdateChecked:t,"onUpdate:checked":n}=e,{nTriggerFormInput:r,nTriggerFormChange:a}=i;t&&m(t,!0),n&&m(n,!0),r(),a(),l.value=!0}}function y(){o.value||p.value||_()}function x(){y(),s.value&&(s.value.checked=p.value)}function S(){g.value=!1}function w(){g.value=!0}return{mergedClsPrefix:t?t.mergedClsPrefixRef:n,inputRef:s,labelRef:c,mergedName:h,mergedDisabled:o,renderSafeChecked:p,focus:g,mergedSize:a,handleRadioInputChange:x,handleRadioInputBlur:S,handleRadioInputFocus:w}}var L=[`value`,`name`,`checked`,`disabled`,`onChange`,`onFocus`,`onBlur`],R={...r.props,...P},z=y({name:`Radio`,props:R,setup(e){let t=I(e),n=r(`Radio`,`-radio`,N,g,e,t.mergedClsPrefix),i=w(()=>{let{mergedSize:{value:e}}=t,{common:{cubicBezierEaseInOut:r},self:{boxShadow:i,boxShadowActive:a,boxShadowDisabled:o,boxShadowFocus:s,boxShadowHover:c,color:l,colorDisabled:u,colorActive:f,textColor:p,textColorDisabled:m,dotColorActive:h,dotColorDisabled:g,labelPadding:_,labelLineHeight:v,labelFontWeight:y,[d(`fontSize`,e)]:b,[d(`radioSize`,e)]:x}}=n.value;return{"--n-bezier":r,"--n-label-line-height":v,"--n-label-font-weight":y,"--n-box-shadow":i,"--n-box-shadow-active":a,"--n-box-shadow-disabled":o,"--n-box-shadow-focus":s,"--n-box-shadow-hover":c,"--n-color":l,"--n-color-active":f,"--n-color-disabled":u,"--n-dot-color-active":h,"--n-dot-color-disabled":g,"--n-font-size":b,"--n-radio-size":x,"--n-text-color":p,"--n-text-color-disabled":m,"--n-label-padding":_}}),{inlineThemeDisabled:a,mergedClsPrefixRef:o,mergedRtlRef:s}=f(e),l=h(`Radio`,s,o),u=a?c(`radio`,w(()=>t.mergedSize.value[0]),i,e):void 0;return Object.assign(t,{rtlEnabled:l,cssVars:a?void 0:i,themeClass:u?.themeClass,onRender:u?.onRender})},render(){let{$slots:e,mergedClsPrefix:n,onRender:r,label:i}=this;return r?.(),(()=>{let r=o(`f8c6901d8cd45c02`);return x(),E(`label`,{class:a([`${n}-radio`,this.themeClass,this.rtlEnabled&&`${n}-radio--rtl`,this.mergedDisabled&&`${n}-radio--disabled`,this.renderSafeChecked&&`${n}-radio--checked`,this.focus&&`${n}-radio--focus`]),style:S(this.cssVars)},[T(`div`,{class:a(`${n}-radio__dot-wrapper`)},[r[0]||(r[0]=p(`\xA0`,-1)),T(`div`,{class:a([`${n}-radio__dot`,this.renderSafeChecked&&`${n}-radio__dot--checked`])},null,2),T(`input`,{ref:`inputRef`,type:`radio`,class:a(`${n}-radio-input`),value:this.value,name:this.mergedName,checked:this.renderSafeChecked,disabled:this.mergedDisabled,onChange:this.handleRadioInputChange,onFocus:this.handleRadioInputFocus,onBlur:this.handleRadioInputBlur},null,42,L)],2),p(()=>t(e.default,e=>!e&&!i?null:(x(),E(`div`,{ref:`labelRef`,class:a(`${n}-radio__label`)},[p(()=>e||i)],2))))],6)})()}}),B=n(`radio-group`,`
 display: inline-block;
 font-size: var(--n-font-size);
`,[u(`splitor`,`
 display: inline-block;
 vertical-align: bottom;
 width: 1px;
 transition:
 background-color .3s var(--n-bezier),
 opacity .3s var(--n-bezier);
 background: var(--n-button-border-color);
 `,[i(`checked`,{backgroundColor:`var(--n-button-border-color-active)`}),i(`disabled`,{opacity:`var(--n-opacity-disabled)`})]),i(`button-group`,`
 white-space: nowrap;
 height: var(--n-height);
 line-height: var(--n-height);
 `,[n(`radio-button`,{height:`var(--n-height)`,lineHeight:`var(--n-height)`}),u(`splitor`,{height:`var(--n-height)`})]),n(`radio-button`,`
 vertical-align: bottom;
 outline: none;
 position: relative;
 user-select: none;
 -webkit-user-select: none;
 display: inline-block;
 box-sizing: border-box;
 padding-left: 14px;
 padding-right: 14px;
 white-space: nowrap;
 transition:
 background-color .3s var(--n-bezier),
 opacity .3s var(--n-bezier),
 border-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
 background: var(--n-button-color);
 color: var(--n-button-text-color);
 border-top: 1px solid var(--n-button-border-color);
 border-bottom: 1px solid var(--n-button-border-color);
 `,[n(`radio-input`,`
 pointer-events: none;
 position: absolute;
 border: 0;
 border-radius: inherit;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 opacity: 0;
 z-index: 1;
 `),u(`state-border`,`
 z-index: 1;
 pointer-events: none;
 position: absolute;
 box-shadow: var(--n-button-box-shadow);
 transition: box-shadow .3s var(--n-bezier);
 left: -1px;
 bottom: -1px;
 right: -1px;
 top: -1px;
 `),s(`&:first-child`,`
 border-top-left-radius: var(--n-button-border-radius);
 border-bottom-left-radius: var(--n-button-border-radius);
 border-left: 1px solid var(--n-button-border-color);
 `,[u(`state-border`,`
 border-top-left-radius: var(--n-button-border-radius);
 border-bottom-left-radius: var(--n-button-border-radius);
 `)]),s(`&:last-child`,`
 border-top-right-radius: var(--n-button-border-radius);
 border-bottom-right-radius: var(--n-button-border-radius);
 border-right: 1px solid var(--n-button-border-color);
 `,[u(`state-border`,`
 border-top-right-radius: var(--n-button-border-radius);
 border-bottom-right-radius: var(--n-button-border-radius);
 `)]),l(`disabled`,`
 cursor: pointer;
 `,[s(`&:hover`,[u(`state-border`,`
 transition: box-shadow .3s var(--n-bezier);
 box-shadow: var(--n-button-box-shadow-hover);
 `),l(`checked`,{color:`var(--n-button-text-color-hover)`})]),i(`focus`,[s(`&:not(:active)`,[u(`state-border`,{boxShadow:`var(--n-button-box-shadow-focus)`})])])]),i(`checked`,`
 background: var(--n-button-color-active);
 color: var(--n-button-text-color-active);
 border-color: var(--n-button-border-color-active);
 `),i(`disabled`,`
 cursor: not-allowed;
 opacity: var(--n-opacity-disabled);
 `)])]),V=[`onFocusin`,`onFocusout`];function H(e,t,n){let r=[],i=!1;for(let o=0;o<e.length;++o){let s=e[o],c=s.type?.name;c===`RadioButton`&&(i=!0);let l=s.props;if(c!==`RadioButton`)r.push(s);else if(o===0)r.push(s);else{let e=r[r.length-1].props,i=t===e.value,o=e.disabled,c=t===l.value,u=l.disabled,d=(i?2:0)+ +!o,f=(c?2:0)+ +!u,p={[`${n}-radio-group__splitor--disabled`]:o,[`${n}-radio-group__splitor--checked`]:i},m={[`${n}-radio-group__splitor--disabled`]:u,[`${n}-radio-group__splitor--checked`]:c},h=d<f?m:p;r.push((x(),E(`div`,{key:1,class:a([`${n}-radio-group__splitor`,h])},null,2)),s)}}return{children:r,isButtonGroup:i}}var U={...r.props,name:String,options:Array,labelField:{type:String,default:`label`},valueField:{type:String,default:`value`},value:[String,Number,Boolean],defaultValue:{type:[String,Number,Boolean],default:null},size:String,disabled:{type:Boolean,default:void 0},"onUpdate:value":[Function,Array],onUpdateValue:[Function,Array]},W=y({name:`RadioGroup`,props:U,setup(e){let t=C(null),{mergedSizeRef:n,mergedDisabledRef:i,nTriggerFormChange:a,nTriggerFormInput:o,nTriggerFormBlur:s,nTriggerFormFocus:l}=O(e),{mergedClsPrefixRef:u,inlineThemeDisabled:p,mergedRtlRef:v}=f(e),y=r(`Radio`,`-radio-group`,B,g,e,u),x=C(e.defaultValue),S=b(e,`value`),T=A(S,x);function E(t){let{onUpdateValue:n,"onUpdate:value":r}=e;n&&m(n,t),r&&m(r,t),x.value=t,a(),o()}function D(e){let{value:n}=t;n&&(n.contains(e.relatedTarget)||l())}function k(e){let{value:n}=t;n&&(n.contains(e.relatedTarget)||s())}_(F,{mergedClsPrefixRef:u,nameRef:b(e,`name`),valueRef:T,disabledRef:i,mergedSizeRef:n,doUpdateValue:E});let j=h(`Radio`,v,u),M=w(()=>{let{value:e}=n,{common:{cubicBezierEaseInOut:t},self:{buttonBorderColor:r,buttonBorderColorActive:i,buttonBorderRadius:a,buttonBoxShadow:o,buttonBoxShadowFocus:s,buttonBoxShadowHover:c,buttonColor:l,buttonColorActive:u,buttonTextColor:f,buttonTextColorActive:p,buttonTextColorHover:m,opacityDisabled:h,[d(`buttonHeight`,e)]:g,[d(`fontSize`,e)]:_}}=y.value;return{"--n-font-size":_,"--n-bezier":t,"--n-button-border-color":r,"--n-button-border-color-active":i,"--n-button-border-radius":a,"--n-button-box-shadow":o,"--n-button-box-shadow-focus":s,"--n-button-box-shadow-hover":c,"--n-button-color":l,"--n-button-color-active":u,"--n-button-text-color":f,"--n-button-text-color-hover":m,"--n-button-text-color-active":p,"--n-height":g,"--n-opacity-disabled":h}}),N=p?c(`radio-group`,w(()=>n.value[0]),M,e):void 0;return{selfElRef:t,rtlEnabled:j,mergedClsPrefix:u,mergedValue:T,handleFocusout:k,handleFocusin:D,cssVars:p?void 0:M,themeClass:N?.themeClass,onRender:N?.onRender}},render(){let{mergedValue:e,mergedClsPrefix:t,handleFocusin:n,handleFocusout:r}=this,{options:i,labelField:o,valueField:s}=this.$props,{children:c,isButtonGroup:l}=H(i?i.map(e=>{let t=e[s];return x(),D(z,{key:typeof t==`boolean`?`__n_${t}`:t,value:t,disabled:e.disabled,label:e[o]},null,8,[`value`,`disabled`,`label`])}):j(M(this)),e,t);return this.onRender?.(),x(),E(`div`,{onFocusin:n,onFocusout:r,ref:`selfElRef`,class:a([`${t}-radio-group`,this.rtlEnabled&&`${t}-radio-group--rtl`,this.themeClass,l&&`${t}-radio-group--button-group`]),style:S(this.cssVars)},[p(()=>c)],46,V)}});export{I as i,z as n,P as r,W as t};