import{at as e,c as t,ct as n,dt as r,ft as i,g as a,it as o,l as s,lt as c,st as l,ut as u,w as d,y as f}from"./resolve-slot-D0ESJhJD.js";import{gt as p}from"./user-B1sGzif1.js";import{S as m,V as h,Wt as g,d as _,f as v,h as y}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{n as b}from"./misc-DDs3MKLt.js";import{t as x}from"./use-compitable-CgN8vuBj.js";import{t as S}from"./flatten-q_1Pdy7y.js";import{t as C}from"./get-slot-6kXJmSMP.js";function w(e,t=`default`,n=[]){let{children:r}=e;if(typeof r==`object`&&r&&!Array.isArray(r)){let e=r[t];if(typeof e==`function`)return e()}return n}var T=o([e(`descriptions`,{fontSize:`var(--n-font-size)`},[e(`descriptions-separator`,`
 display: inline-block;
 margin: 0 8px 0 2px;
 `),e(`descriptions-table-wrapper`,[e(`descriptions-table`,[e(`descriptions-table-row`,[e(`descriptions-table-header`,{padding:`var(--n-th-padding)`}),e(`descriptions-table-content`,{padding:`var(--n-td-padding)`})])])]),c(`bordered`,[e(`descriptions-table-wrapper`,[e(`descriptions-table`,[e(`descriptions-table-row`,[o(`&:last-child`,[e(`descriptions-table-content`,{paddingBottom:0})])])])])]),n(`left-label-placement`,[e(`descriptions-table-content`,[o(`> *`,{verticalAlign:`top`})])]),n(`left-label-align`,[o(`th`,{textAlign:`left`})]),n(`center-label-align`,[o(`th`,{textAlign:`center`})]),n(`right-label-align`,[o(`th`,{textAlign:`right`})]),n(`bordered`,[e(`descriptions-table-wrapper`,`
 border-radius: var(--n-border-radius);
 overflow: hidden;
 background: var(--n-merged-td-color);
 border: 1px solid var(--n-merged-border-color);
 `,[e(`descriptions-table`,[e(`descriptions-table-row`,[o(`&:not(:last-child)`,[e(`descriptions-table-content`,{borderBottom:`1px solid var(--n-merged-border-color)`}),e(`descriptions-table-header`,{borderBottom:`1px solid var(--n-merged-border-color)`})]),e(`descriptions-table-header`,`
 font-weight: 400;
 background-clip: padding-box;
 background-color: var(--n-merged-th-color);
 `,[o(`&:not(:last-child)`,{borderRight:`1px solid var(--n-merged-border-color)`})]),e(`descriptions-table-content`,[o(`&:not(:last-child)`,{borderRight:`1px solid var(--n-merged-border-color)`})])])])])]),e(`descriptions-header`,`
 font-weight: var(--n-th-font-weight);
 font-size: 18px;
 transition: color .3s var(--n-bezier);
 line-height: var(--n-line-height);
 margin-bottom: 16px;
 color: var(--n-title-text-color);
 `),e(`descriptions-table-wrapper`,`
 transition:
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `,[e(`descriptions-table`,`
 width: 100%;
 border-collapse: separate;
 border-spacing: 0;
 box-sizing: border-box;
 `,[e(`descriptions-table-row`,`
 box-sizing: border-box;
 transition: border-color .3s var(--n-bezier);
 `,[e(`descriptions-table-header`,`
 font-weight: var(--n-th-font-weight);
 line-height: var(--n-line-height);
 display: table-cell;
 box-sizing: border-box;
 color: var(--n-th-text-color);
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `),e(`descriptions-table-content`,`
 vertical-align: top;
 line-height: var(--n-line-height);
 display: table-cell;
 box-sizing: border-box;
 color: var(--n-td-text-color);
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `,[l(`content`,`
 transition: color .3s var(--n-bezier);
 display: inline-block;
 color: var(--n-td-text-color);
 `)]),l(`label`,`
 font-weight: var(--n-th-font-weight);
 transition: color .3s var(--n-bezier);
 display: inline-block;
 margin-right: 14px;
 color: var(--n-th-text-color);
 `)])])])]),e(`descriptions-table-wrapper`,`
 --n-merged-th-color: var(--n-th-color);
 --n-merged-td-color: var(--n-td-color);
 --n-merged-border-color: var(--n-border-color);
 `),r(e(`descriptions-table-wrapper`,`
 --n-merged-th-color: var(--n-th-color-modal);
 --n-merged-td-color: var(--n-td-color-modal);
 --n-merged-border-color: var(--n-border-color-modal);
 `)),i(e(`descriptions-table-wrapper`,`
 --n-merged-th-color: var(--n-th-color-popover);
 --n-merged-td-color: var(--n-td-color-popover);
 --n-merged-border-color: var(--n-border-color-popover);
 `))]),E=`DESCRIPTION_ITEM_FLAG`;function D(e){return typeof e==`object`&&e&&!Array.isArray(e)?e.type&&e.type.DESCRIPTION_ITEM_FLAG:!1}var O=[`colspan`],k=[`colspan`],A=[`colspan`],j=[`colspan`],M={...t.props,title:String,column:{type:Number,default:3},columns:Number,labelPlacement:{type:String,default:`top`},labelAlign:{type:String,default:`left`},separator:{type:String,default:`:`},size:String,bordered:Boolean,labelClass:String,labelStyle:[Object,String],contentClass:String,contentStyle:[Object,String]},N=m({name:`Descriptions`,props:M,slots:Object,setup(e){let{mergedClsPrefixRef:n,inlineThemeDisabled:r,mergedComponentPropsRef:i}=d(e),a=_(()=>e.size||i?.value?.Descriptions?.size||`medium`),o=t(`Descriptions`,`-descriptions`,T,p,e,n),c=_(()=>{let{bordered:t}=e,n=a.value,{common:{cubicBezierEaseInOut:r},self:{titleTextColor:i,thColor:s,thColorModal:c,thColorPopover:l,thTextColor:d,thFontWeight:f,tdTextColor:p,tdColor:m,tdColorModal:h,tdColorPopover:g,borderColor:_,borderColorModal:v,borderColorPopover:y,borderRadius:b,lineHeight:x,[u(`fontSize`,n)]:S,[u(t?`thPaddingBordered`:`thPadding`,n)]:C,[u(t?`tdPaddingBordered`:`tdPadding`,n)]:w}}=o.value;return{"--n-title-text-color":i,"--n-th-padding":C,"--n-td-padding":w,"--n-font-size":S,"--n-bezier":r,"--n-th-font-weight":f,"--n-line-height":x,"--n-th-text-color":d,"--n-td-text-color":p,"--n-th-color":s,"--n-th-color-modal":c,"--n-th-color-popover":l,"--n-td-color":m,"--n-td-color-modal":h,"--n-td-color-popover":g,"--n-border-radius":b,"--n-border-color":_,"--n-border-color-modal":v,"--n-border-color-popover":y}}),l=r?s(`descriptions`,_(()=>{let t=``,{bordered:n}=e;return n&&(t+=`a`),t+=a.value[0],t}),c,e):void 0;return{mergedClsPrefix:n,cssVars:r?void 0:c,themeClass:l?.themeClass,onRender:l?.onRender,compitableColumn:x(e,[`columns`,`column`]),inlineThemeDisabled:r,mergedSize:a}},render(){let e=this.$slots.default,t=e?S(e()):[];t.length;let{contentClass:n,labelClass:r,compitableColumn:i,labelPlacement:o,labelAlign:s,mergedSize:c,bordered:l,title:u,cssVars:d,mergedClsPrefix:p,separator:m,onRender:_}=this;_?.();let x=t.filter(e=>D(e)),T=x.reduce((e,t,s)=>{let c=t.props||{},u=x.length-1===s,d=[`label`in c?c.label:w(t,`label`)],_=[w(t)],b=c.span||1,S=e.span;e.span+=b;let C=c.labelStyle||c[`label-style`]||this.labelStyle,T=c.contentStyle||c[`content-style`]||this.contentStyle;if(o===`left`)l?e.row.push((h(),y(`th`,{key:1,class:a([`${p}-descriptions-table-header`,r]),colspan:1,style:g(C)},[f(()=>d)],6)),(h(),y(`td`,{key:2,class:a([`${p}-descriptions-table-content`,n]),colspan:u?(i-S)*2+1:b*2-1,style:g(T)},[f(()=>_)],14,O))):e.row.push((h(),y(`td`,{key:3,class:a(`${p}-descriptions-table-content`),colspan:u?(i-S)*2:b*2},[v(`span`,{class:a([`${p}-descriptions-table-content__label`,r]),style:g(C)},[f(()=>[...d,m&&(h(),y(`span`,{key:4,class:a(`${p}-descriptions-separator`)},[f(()=>m)],2))])],6),v(`span`,{class:a([`${p}-descriptions-table-content__content`,n]),style:g(T)},[f(()=>_)],6)],10,k)));else{let t=u?(i-S)*2:b*2;e.row.push((h(),y(`th`,{key:5,class:a([`${p}-descriptions-table-header`,r]),colspan:t,style:g(C)},[f(()=>d)],14,A))),e.secondRow.push((h(),y(`td`,{key:6,class:a([`${p}-descriptions-table-content`,n]),colspan:t,style:g(T)},[f(()=>_)],14,j)))}return(e.span>=i||u)&&(e.span=0,e.row.length&&(e.rows.push(e.row),e.row=[]),o!==`left`&&e.secondRow.length&&(e.rows.push(e.secondRow),e.secondRow=[])),e},{span:0,row:[],secondRow:[],rows:[]}).rows.map(e=>(h(),y(`tr`,{class:a(`${p}-descriptions-table-row`)},[f(()=>e)],2)));return h(),y(`div`,{style:g(d),class:a([`${p}-descriptions`,this.themeClass,`${p}-descriptions--${o}-label-placement`,`${p}-descriptions--${s}-label-align`,`${p}-descriptions--${c}-size`,l&&`${p}-descriptions--bordered`])},[u||this.$slots.header?(h(),y(`div`,{key:0,class:a(`${p}-descriptions-header`)},[f(()=>u||C(this,`header`))],2)):f(()=>null),v(`div`,{class:a(`${p}-descriptions-table-wrapper`)},[v(`table`,{class:a(`${p}-descriptions-table`)},[v(`tbody`,null,[f(()=>o===`top`&&(h(),y(`tr`,{class:a(`${p}-descriptions-table-row`),style:{visibility:`collapse`}},[f(()=>b(i*2,(h(),y(`td`))))],2))),f(()=>T)])],2)],2)],6)}}),P={label:String,span:{type:Number,default:1},labelClass:String,labelStyle:[Object,String],contentClass:String,contentStyle:[Object,String]},F=m({name:`DescriptionsItem`,[E]:!0,props:P,slots:Object,render(){return null}});export{N as n,F as t};