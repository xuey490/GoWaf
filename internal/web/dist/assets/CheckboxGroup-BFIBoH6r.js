import{D as e,a as t,at as n,c as r,ct as i,dt as a,ft as o,g as s,h as c,it as l,l as u,st as d,ut as f,w as p,y as m}from"./resolve-slot-D0ESJhJD.js";import{n as h,t as g}from"./use-rtl-B41EZXdW.js";import{H as _,O as v,S as y,St as b,V as x,Wt as S,_t as C,b as w,d as T,f as E,h as D,i as O,p as k}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{t as A}from"./misc-DDs3MKLt.js";import{n as j}from"./delegate-C-o38LzM.js";import{i as M,n as N,o as P,t as F}from"./icon-switch.cssr-o4i5UUsg.js";import{t as I}from"./use-merged-state-KQOIh2IK.js";import{t as L}from"./light-DiIraETQ.js";var R=()=>(()=>{let e=c(`75be776d8875fa17`);return e[0]||(e[0]=E(`svg`,{viewBox:`0 0 64 64`,class:`check-icon`},[E(`path`,{d:`M50.42,16.76L22.34,39.45l-8.1-11.46c-1.12-1.58-3.3-1.96-4.88-0.84c-1.58,1.12-1.95,3.3-0.84,4.88l10.26,14.51  c0.56,0.79,1.42,1.31,2.38,1.45c0.16,0.02,0.32,0.03,0.48,0.03c0.8,0,1.57-0.27,2.2-0.78l30.99-25.03c1.5-1.21,1.74-3.42,0.52-4.92  C54.13,15.78,51.93,15.55,50.42,16.76z`})],-1))})(),z=()=>(()=>{let e=c(`c6eed899356c8404`);return e[0]||(e[0]=E(`svg`,{viewBox:`0 0 100 100`,class:`line-icon`},[E(`path`,{d:`M80.2,55.5H21.4c-2.8,0-5.1-2.5-5.1-5.5l0,0c0-3,2.3-5.5,5.1-5.5h58.7c2.8,0,5.1,2.5,5.1,5.5l0,0C85.2,53.1,82.9,55.5,80.2,55.5z`})],-1))})(),B=l([n(`checkbox`,`
 font-size: var(--n-font-size);
 outline: none;
 cursor: pointer;
 display: inline-flex;
 flex-wrap: nowrap;
 align-items: flex-start;
 word-break: break-word;
 line-height: var(--n-size);
 --n-merged-color-table: var(--n-color-table);
 `,[i(`show-label`,`line-height: var(--n-label-line-height);`),l(`&:hover`,[n(`checkbox-box`,[d(`border`,`border: var(--n-border-checked);`)])]),l(`&:focus:not(:active)`,[n(`checkbox-box`,[d(`border`,`
 border: var(--n-border-focus);
 box-shadow: var(--n-box-shadow-focus);
 `)])]),i(`inside-table`,[n(`checkbox-box`,`
 background-color: var(--n-merged-color-table);
 `)]),i(`checked`,[n(`checkbox-box`,`
 background-color: var(--n-color-checked);
 `,[n(`checkbox-icon`,[l(`.check-icon`,`
 opacity: 1;
 transform: scale(1);
 `)])])]),i(`indeterminate`,[n(`checkbox-box`,[n(`checkbox-icon`,[l(`.check-icon`,`
 opacity: 0;
 transform: scale(.5);
 `),l(`.line-icon`,`
 opacity: 1;
 transform: scale(1);
 `)])])]),i(`checked, indeterminate`,[l(`&:focus:not(:active)`,[n(`checkbox-box`,[d(`border`,`
 border: var(--n-border-checked);
 box-shadow: var(--n-box-shadow-focus);
 `)])]),n(`checkbox-box`,`
 background-color: var(--n-color-checked);
 border-left: 0;
 border-top: 0;
 `,[d(`border`,{border:`var(--n-border-checked)`})])]),i(`disabled`,{cursor:`not-allowed`},[i(`checked`,[n(`checkbox-box`,`
 background-color: var(--n-color-disabled-checked);
 `,[d(`border`,{border:`var(--n-border-disabled-checked)`}),n(`checkbox-icon`,[l(`.check-icon, .line-icon`,{fill:`var(--n-check-mark-color-disabled-checked)`})])])]),n(`checkbox-box`,`
 background-color: var(--n-color-disabled);
 `,[d(`border`,`
 border: var(--n-border-disabled);
 `),n(`checkbox-icon`,[l(`.check-icon, .line-icon`,`
 fill: var(--n-check-mark-color-disabled);
 `)])]),d(`label`,`
 color: var(--n-text-color-disabled);
 `)]),n(`checkbox-box-wrapper`,`
 position: relative;
 width: var(--n-size);
 flex-shrink: 0;
 flex-grow: 0;
 user-select: none;
 -webkit-user-select: none;
 `),n(`checkbox-box`,`
 position: absolute;
 left: 0;
 top: 50%;
 transform: translateY(-50%);
 height: var(--n-size);
 width: var(--n-size);
 display: inline-block;
 box-sizing: border-box;
 border-radius: var(--n-border-radius);
 background-color: var(--n-color);
 transition: background-color 0.3s var(--n-bezier);
 `,[d(`border`,`
 transition:
 border-color .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier);
 border-radius: inherit;
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 border: var(--n-border);
 `),n(`checkbox-icon`,`
 display: flex;
 align-items: center;
 justify-content: center;
 position: absolute;
 left: 1px;
 right: 1px;
 top: 1px;
 bottom: 1px;
 `,[l(`.check-icon, .line-icon`,`
 width: 100%;
 fill: var(--n-check-mark-color);
 opacity: 0;
 transform: scale(0.5);
 transform-origin: center;
 transition:
 fill 0.3s var(--n-bezier),
 transform 0.3s var(--n-bezier),
 opacity 0.3s var(--n-bezier),
 border-color 0.3s var(--n-bezier);
 `),F({left:`1px`,top:`1px`})])]),d(`label`,`
 color: var(--n-text-color);
 transition: color .3s var(--n-bezier);
 user-select: none;
 -webkit-user-select: none;
 padding: var(--n-label-padding);
 font-weight: var(--n-label-font-weight);
 `,[l(`&:empty`,{display:`none`})])]),a(n(`checkbox`,`
 --n-merged-color-table: var(--n-color-table-modal);
 `)),o(n(`checkbox`,`
 --n-merged-color-table: var(--n-color-table-popover);
 `))]),V=[`id`],H=[`tabindex`,`aria-checked`,`aria-labelledby`,`onKeyup`,`onKeydown`,`onClick`],U={...r.props,size:String,checked:{type:[Boolean,String,Number],default:void 0},defaultChecked:{type:[Boolean,String,Number],default:!1},value:[String,Number],disabled:{type:Boolean,default:void 0},indeterminate:Boolean,label:String,focusable:{type:Boolean,default:!0},checkedValue:{type:[Boolean,String,Number],default:!0},uncheckedValue:{type:[Boolean,String,Number],default:!1},"onUpdate:checked":[Function,Array],onUpdateChecked:[Function,Array],privateInsideTable:Boolean,onChange:[Function,Array]},W=y({name:`Checkbox`,props:U,setup(e){let t=v(G,null),n=C(null),{mergedClsPrefixRef:i,inlineThemeDisabled:a,mergedRtlRef:o,mergedComponentPropsRef:s}=p(e),c=C(e.defaultChecked),l=b(e,`checked`),d=I(l,c),m=P(()=>{if(t){let n=t.valueSetRef.value;return n&&e.value!==void 0?n.has(e.value):!1}return d.value===e.checkedValue}),_=M(e,{mergedSize(n){let{size:r}=e;if(r!==void 0)return r;if(t){let{value:e}=t.mergedSizeRef;if(e!==void 0)return e}if(n){let{mergedSize:e}=n;if(e!==void 0)return e.value}return s?.value?.Checkbox?.size||`medium`},mergedDisabled(n){let{disabled:r}=e;if(r!==void 0)return r;if(t){if(t.disabledRef.value)return!0;let{maxRef:{value:e},checkedCountRef:n}=t;if(e!==void 0&&n.value>=e&&!m.value)return!0;let{minRef:{value:r}}=t;if(r!==void 0&&n.value<=r&&m.value)return!0}return n?n.disabled.value:!1}}),{mergedDisabledRef:y,mergedSizeRef:x}=_,S=r(`Checkbox`,`-checkbox`,B,L,e,i);function w(n){if(t&&e.value!==void 0)t.toggleCheckbox(!m.value,e.value);else{let{onChange:t,"onUpdate:checked":r,onUpdateChecked:i}=e,{nTriggerFormInput:a,nTriggerFormChange:o}=_,s=m.value?e.uncheckedValue:e.checkedValue;r&&h(r,s,n),i&&h(i,s,n),t&&h(t,s,n),a(),o(),c.value=s}}function E(e){y.value||w(e)}function D(e){if(!y.value)switch(e.key){case` `:case`Enter`:w(e)}}function O(e){e.key===` `&&e.preventDefault()}let k={focus:()=>{n.value?.focus()},blur:()=>{n.value?.blur()}},j=g(`Checkbox`,o,i),N=T(()=>{let{value:e}=x,{common:{cubicBezierEaseInOut:t},self:{borderRadius:n,color:r,colorChecked:i,colorDisabled:a,colorTableHeader:o,colorTableHeaderModal:s,colorTableHeaderPopover:c,checkMarkColor:l,checkMarkColorDisabled:u,border:d,borderFocus:p,borderDisabled:m,borderChecked:h,boxShadowFocus:g,textColor:_,textColorDisabled:v,checkMarkColorDisabledChecked:y,colorDisabledChecked:b,borderDisabledChecked:C,labelPadding:w,labelLineHeight:T,labelFontWeight:E,[f(`fontSize`,e)]:D,[f(`size`,e)]:O}}=S.value;return{"--n-label-line-height":T,"--n-label-font-weight":E,"--n-size":O,"--n-bezier":t,"--n-border-radius":n,"--n-border":d,"--n-border-checked":h,"--n-border-focus":p,"--n-border-disabled":m,"--n-border-disabled-checked":C,"--n-box-shadow-focus":g,"--n-color":r,"--n-color-checked":i,"--n-color-table":o,"--n-color-table-modal":s,"--n-color-table-popover":c,"--n-color-disabled":a,"--n-color-disabled-checked":b,"--n-text-color":_,"--n-text-color-disabled":v,"--n-check-mark-color":l,"--n-check-mark-color-disabled":u,"--n-check-mark-color-disabled-checked":y,"--n-font-size":D,"--n-label-padding":w}}),F=a?u(`checkbox`,T(()=>x.value[0]),N,e):void 0;return Object.assign(_,k,{rtlEnabled:j,selfRef:n,mergedClsPrefix:i,mergedDisabled:y,renderedChecked:m,mergedTheme:S,labelId:A(),handleClick:E,handleKeyUp:D,handleKeyDown:O,cssVars:a?void 0:N,themeClass:F?.themeClass,onRender:F?.onRender})},render(){let{$slots:e,renderedChecked:n,mergedDisabled:r,indeterminate:i,privateInsideTable:a,cssVars:o,labelId:l,label:u,mergedClsPrefix:d,focusable:f,handleKeyUp:p,handleKeyDown:h,handleClick:g}=this;this.onRender?.();let _=t(e.default,e=>u||e?(x(),D(`span`,{key:1,class:s(`${d}-checkbox__label`),id:l},[m(()=>u||e)],10,V)):null);return(()=>{let e=c(`70be6e74cd27cb50`);return x(),D(`div`,{ref:`selfRef`,class:s([`${d}-checkbox`,this.themeClass,this.rtlEnabled&&`${d}-checkbox--rtl`,n&&`${d}-checkbox--checked`,r&&`${d}-checkbox--disabled`,i&&`${d}-checkbox--indeterminate`,a&&`${d}-checkbox--inside-table`,_&&`${d}-checkbox--show-label`]),tabindex:r||!f?void 0:0,role:`checkbox`,"aria-checked":i?`mixed`:n,"aria-labelledby":l,style:S(o),onKeyup:p,onKeydown:h,onClick:g,onMousedown:e[0]||(e[0]=()=>{j(`selectstart`,window,e=>{e.preventDefault()},{once:!0})})},[E(`div`,{class:s(`${d}-checkbox-box-wrapper`)},[e[1]||(e[1]=m(`\xA0`,-1)),E(`div`,{class:s(`${d}-checkbox-box`)},[w(N,null,{default:()=>this.indeterminate?(x(),D(`div`,{key:`indeterminate`,class:s(`${d}-checkbox-icon`)},[m(()=>z())],2)):(x(),D(`div`,{key:`check`,class:s(`${d}-checkbox-icon`)},[m(()=>R())],2))},1024),E(`div`,{class:s(`${d}-checkbox-box__border`)},null,2)],2)],2),m(()=>_)],46,H)})()}}),G=e(`n-checkbox-group`),K=y({name:`CheckboxGroup`,props:{min:Number,max:Number,size:String,options:Array,labelField:{type:String,default:`label`},valueField:{type:String,default:`value`},value:Array,defaultValue:{type:Array,default:null},disabled:{type:Boolean,default:void 0},"onUpdate:value":[Function,Array],onUpdateValue:[Function,Array],onChange:[Function,Array]},setup(e){let{mergedClsPrefixRef:t}=p(e),n=M(e),{mergedSizeRef:r,mergedDisabledRef:i}=n,a=C(e.defaultValue),o=T(()=>e.value),s=I(o,a),c=T(()=>s.value?.length||0),l=T(()=>Array.isArray(s.value)?new Set(s.value):new Set);function u(t,r){let{nTriggerFormInput:i,nTriggerFormChange:o}=n,{onChange:c,"onUpdate:value":l,onUpdateValue:u}=e;if(Array.isArray(s.value)){let e=Array.from(s.value),n=e.findIndex(e=>e===r);t?~n||(e.push(r),u&&h(u,e,{actionType:`check`,value:r}),l&&h(l,e,{actionType:`check`,value:r}),i(),o(),a.value=e,c&&h(c,e)):~n&&(e.splice(n,1),u&&h(u,e,{actionType:`uncheck`,value:r}),l&&h(l,e,{actionType:`uncheck`,value:r}),c&&h(c,e),a.value=e,i(),o())}else t?(u&&h(u,[r],{actionType:`check`,value:r}),l&&h(l,[r],{actionType:`check`,value:r}),c&&h(c,[r]),a.value=[r],i(),o()):(u&&h(u,[],{actionType:`uncheck`,value:r}),l&&h(l,[],{actionType:`uncheck`,value:r}),c&&h(c,[]),a.value=[],i(),o())}return _(G,{checkedCountRef:c,maxRef:b(e,`max`),minRef:b(e,`min`),valueSetRef:l,disabledRef:i,mergedSizeRef:r,toggleCheckbox:u}),{mergedClsPrefix:t}},render(){let{options:e,labelField:t,valueField:n}=this.$props;return x(),D(`div`,{class:s(`${this.mergedClsPrefix}-checkbox-group`),role:`group`},[e?(x(),D(O,{key:0},[m(()=>e.map(e=>{let r=e[n];return x(),k(W,{key:r,value:r,disabled:e.disabled,label:e[t]},null,8,[`value`,`disabled`,`label`])}))],64)):(x(),D(O,{key:1},[m(()=>this.$slots.default?.())],64))],2)}});export{W as n,K as t};