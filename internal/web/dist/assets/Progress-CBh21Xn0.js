import{at as e,c as t,ct as n,g as r,it as i,l as a,mt as o,ut as s,v as c,w as l,y as u}from"./resolve-slot-D0ESJhJD.js";import{n as d}from"./replaceable-CG1Qs4lR.js";import{S as f,V as p,Wt as m,d as h,f as g,h as _,i as v,p as y}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{t as b}from"./format-length-BwSo_EUP.js";import{i as x,n as S,r as C,t as w}from"./Warning-DwQXxd7L.js";import{t as T}from"./light-B666qUef.js";var E=[`id`],D=[`stop-color`],O=[`stop-color`],k=[`viewBox`],A=[`d`,`stroke-width`],j=[`d`,`stroke-width`],M={success:(p(),y(S)),error:(p(),y(x)),warning:(p(),y(w)),info:(p(),y(C))},N=f({name:`ProgressCircle`,props:{clsPrefix:{type:String,required:!0},status:{type:String,required:!0},strokeWidth:{type:Number,required:!0},fillColor:[String,Object],railColor:String,railStyle:[String,Object],percentage:{type:Number,default:0},offsetDegree:{type:Number,default:0},showIndicator:{type:Boolean,required:!0},indicatorTextColor:String,unit:String,viewBoxWidth:{type:Number,required:!0},gapDegree:{type:Number,required:!0},gapOffsetDegree:{type:Number,default:0}},setup(e,{slots:t}){let n=h(()=>{let t=`gradient`,{fillColor:n}=e;return typeof n==`object`?`${t}-${o(JSON.stringify(n))}`:t});function i(t,r,i,a){let{gapDegree:o,viewBoxWidth:s,strokeWidth:c}=e,l=50+c/2,u=`M ${l},${l} m 0,50
      a 50,50 0 1 1 0,-100
      a 50,50 0 1 1 0,100`,d=Math.PI*2*50;return{pathString:u,pathStyle:{stroke:a===`rail`?i:typeof e.fillColor==`object`?`url(#${n.value})`:i,strokeDasharray:`${Math.min(t,100)/100*(d-o)}px ${s*8}px`,strokeDashoffset:`-${o/2}px`,transformOrigin:r?`center`:void 0,transform:r?`rotate(${r}deg)`:void 0}}}let a=()=>{let t=typeof e.fillColor==`object`,r=t?e.fillColor.stops[0]:``,i=t?e.fillColor.stops[1]:``;return t&&(p(),_(`defs`,null,[g(`linearGradient`,{id:n.value,x1:`0%`,y1:`100%`,x2:`100%`,y2:`0%`},[g(`stop`,{offset:`0%`,"stop-color":r},null,8,D),g(`stop`,{offset:`100%`,"stop-color":i},null,8,O)],8,E)]))};return()=>{let{fillColor:n,railColor:o,strokeWidth:s,offsetDegree:c,status:l,percentage:f,showIndicator:h,indicatorTextColor:b,unit:x,gapOffsetDegree:S,clsPrefix:C}=e,{pathString:w,pathStyle:T}=i(100,0,o,`rail`),{pathString:E,pathStyle:D}=i(f,c,n,`fill`),O=100+s;return p(),_(`div`,{class:r(`${C}-progress-content`),role:`none`},[g(`div`,{class:r(`${C}-progress-graph`),"aria-hidden":!0},[g(`div`,{class:r(`${C}-progress-graph-circle`),style:m({transform:S?`rotate(${S}deg)`:void 0})},[(p(),_(`svg`,{viewBox:`0 0 ${O} ${O}`},[u(()=>a()),g(`g`,null,[g(`path`,{class:r(`${C}-progress-graph-circle-rail`),d:w,"stroke-width":s,"stroke-linecap":`round`,fill:`none`,style:m(T)},null,14,A)]),g(`g`,null,[g(`path`,{class:r([`${C}-progress-graph-circle-fill`,f===0&&`${C}-progress-graph-circle-fill--empty`]),d:E,"stroke-width":s,"stroke-linecap":`round`,fill:`none`,style:m(D)},null,14,j)])],8,k))],6)],2),h?(p(),_(`div`,{key:0},[t.default?(p(),_(`div`,{key:0,class:r(`${C}-progress-custom-content`),role:`none`},[u(()=>t.default())],2)):(p(),_(v,{key:1},[l==="default"?(p(),_(`div`,{key:1,class:r(`${C}-progress-text`),style:m({color:b}),role:`none`},[g(`span`,{class:r(`${C}-progress-text__percentage`)},[u(()=>f)],2),g(`span`,{class:r(`${C}-progress-text__unit`)},[u(()=>x)],2)],6)):(p(),_(`div`,{key:0,class:r(`${C}-progress-icon`),"aria-hidden":!0},[(p(),y(d,{clsPrefix:C},{default:()=>M[l]},1032,[`clsPrefix`]))],2))],64))])):u(()=>null)],2)}}}),P={success:(p(),y(S)),error:(p(),y(x)),warning:(p(),y(w)),info:(p(),y(C))},F=f({name:`ProgressLine`,props:{clsPrefix:{type:String,required:!0},percentage:{type:Number,default:0},railColor:String,railStyle:[String,Object],fillColor:[String,Object],status:{type:String,required:!0},indicatorPlacement:{type:String,required:!0},indicatorTextColor:String,unit:{type:String,default:`%`},processing:{type:Boolean,required:!0},showIndicator:{type:Boolean,required:!0},height:[String,Number],railBorderRadius:[String,Number],fillBorderRadius:[String,Number]},setup(e,{slots:t}){let n=h(()=>b(e.height)),i=h(()=>typeof e.fillColor==`object`?`linear-gradient(to right, ${e.fillColor?.stops[0]} , ${e.fillColor?.stops[1]})`:e.fillColor),a=h(()=>e.railBorderRadius===void 0?e.height===void 0?``:b(e.height,{c:.5}):b(e.railBorderRadius)),o=h(()=>e.fillBorderRadius===void 0?e.railBorderRadius===void 0?e.height===void 0?``:b(e.height,{c:.5}):b(e.railBorderRadius):b(e.fillBorderRadius));return()=>{let{indicatorPlacement:s,railColor:c,railStyle:l,percentage:f,unit:h,indicatorTextColor:b,status:x,showIndicator:S,processing:C,clsPrefix:w}=e;return p(),_(`div`,{class:r(`${w}-progress-content`),role:`none`},[g(`div`,{class:r(`${w}-progress-graph`),"aria-hidden":!0},[g(`div`,{class:r([`${w}-progress-graph-line`,{[`${w}-progress-graph-line--indicator-${s}`]:!0}])},[g(`div`,{class:r(`${w}-progress-graph-line-rail`),style:m([{backgroundColor:c,height:n.value,borderRadius:a.value},l])},[g(`div`,{class:r([`${w}-progress-graph-line-fill`,C&&`${w}-progress-graph-line-fill--processing`]),style:m({maxWidth:`${e.percentage}%`,background:i.value,height:n.value,lineHeight:n.value,borderRadius:o.value})},[s===`inside`?(p(),_(`div`,{key:0,class:r(`${w}-progress-graph-line-indicator`),style:m({color:b})},[t.default?(p(),_(v,{key:0},[u(()=>t.default())],64)):(p(),_(v,{key:1},[u(()=>`${f}${h}`)],64))],6)):u(()=>null)],6)],6)],2)],2),S&&s===`outside`?(p(),_(`div`,{key:0},[t.default?(p(),_(`div`,{key:0,class:r(`${w}-progress-custom-content`),style:m({color:b}),role:`none`},[u(()=>t.default())],6)):(p(),_(v,{key:1},[x==="default"?(p(),_(`div`,{key:0,role:`none`,class:r(`${w}-progress-icon ${w}-progress-icon--as-text`),style:m({color:b})},[u(()=>f),u(()=>h)],6)):(p(),_(`div`,{key:1,class:r(`${w}-progress-icon`),"aria-hidden":!0},[(p(),y(d,{clsPrefix:w},{default:()=>P[x]},1032,[`clsPrefix`]))],2))],64))])):u(()=>null)],2)}}}),I=[`id`],L=[`stop-color`],R=[`stop-color`],z=[`d`,`stroke-width`],B=[`d`,`stroke-width`],V=[`viewBox`];function H(e,t,n=100){return`m ${n/2} ${n/2-e} a ${e} ${e} 0 1 1 0 ${2*e} a ${e} ${e} 0 1 1 0 -${2*e}`}var U=f({name:`ProgressMultipleCircle`,props:{clsPrefix:{type:String,required:!0},viewBoxWidth:{type:Number,required:!0},percentage:{type:Array,default:[0]},strokeWidth:{type:Number,required:!0},circleGap:{type:Number,required:!0},showIndicator:{type:Boolean,required:!0},fillColor:{type:Array,default:()=>[]},railColor:{type:Array,default:()=>[]},railStyle:{type:Array,default:()=>[]}},setup(e,{slots:t}){let n=h(()=>e.percentage.map((t,n)=>`${Math.PI*t/100*(e.viewBoxWidth/2-e.strokeWidth/2*(1+2*n)-e.circleGap*n)*2}, ${e.viewBoxWidth*8}`)),i=(t,n)=>{let r=e.fillColor[n],i=typeof r==`object`?r.stops[0]:``,a=typeof r==`object`?r.stops[1]:``;return typeof e.fillColor[n]==`object`&&(p(),_(`linearGradient`,{id:`gradient-${n}`,x1:`100%`,y1:`0%`,x2:`0%`,y2:`100%`},[g(`stop`,{offset:`0%`,"stop-color":i},null,8,L),g(`stop`,{offset:`100%`,"stop-color":a},null,8,R)],8,I))};return()=>{let{viewBoxWidth:a,strokeWidth:o,circleGap:s,showIndicator:c,fillColor:l,railColor:d,railStyle:f,percentage:h,clsPrefix:v}=e;return p(),_(`div`,{class:r(`${v}-progress-content`),role:`none`},[g(`div`,{class:r(`${v}-progress-graph`),"aria-hidden":!0},[g(`div`,{class:r(`${v}-progress-graph-circle`)},[(p(),_(`svg`,{viewBox:`0 0 ${a} ${a}`},[g(`defs`,null,[u(()=>h.map((e,t)=>i(e,t)))]),u(()=>h.map((e,t)=>(p(),_(`g`,{key:t},[g(`path`,{class:r(`${v}-progress-graph-circle-rail`),d:H(a/2-o/2*(1+2*t)-s*t,o,a),"stroke-width":o,"stroke-linecap":`round`,fill:`none`,style:m([{strokeDashoffset:0,stroke:d[t]},f[t]])},null,14,z),g(`path`,{class:r([`${v}-progress-graph-circle-fill`,e===0&&`${v}-progress-graph-circle-fill--empty`]),d:H(a/2-o/2*(1+2*t)-s*t,o,a),"stroke-width":o,"stroke-linecap":`round`,fill:`none`,style:m({strokeDasharray:n.value[t],strokeDashoffset:0,stroke:typeof l[t]==`object`?`url(#gradient-${t})`:l[t]})},null,14,B)]))))],8,V))],2)],2),c&&t.default?(p(),_(`div`,{key:0},[g(`div`,{class:r(`${v}-progress-text`)},[u(()=>t.default())],2)])):u(()=>null)],2)}}}),W=i([e(`progress`,{display:`inline-block`},[e(`progress-icon`,`
 color: var(--n-icon-color);
 transition: color .3s var(--n-bezier);
 `),n(`line`,`
 width: 100%;
 display: block;
 `,[e(`progress-content`,`
 display: flex;
 align-items: center;
 `,[e(`progress-graph`,{flex:1})]),e(`progress-custom-content`,{marginLeft:`14px`}),e(`progress-icon`,`
 width: 30px;
 padding-left: 14px;
 height: var(--n-icon-size-line);
 line-height: var(--n-icon-size-line);
 font-size: var(--n-icon-size-line);
 `,[n(`as-text`,`
 color: var(--n-text-color-line-outer);
 text-align: center;
 width: 40px;
 font-size: var(--n-font-size);
 padding-left: 4px;
 transition: color .3s var(--n-bezier);
 `)])]),n(`circle, dashboard`,{width:`120px`},[e(`progress-custom-content`,`
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 display: flex;
 align-items: center;
 justify-content: center;
 `),e(`progress-text`,`
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 display: flex;
 align-items: center;
 color: inherit;
 font-size: var(--n-font-size-circle);
 color: var(--n-text-color-circle);
 font-weight: var(--n-font-weight-circle);
 transition: color .3s var(--n-bezier);
 white-space: nowrap;
 `),e(`progress-icon`,`
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 display: flex;
 align-items: center;
 color: var(--n-icon-color);
 font-size: var(--n-icon-size-circle);
 `)]),n(`multiple-circle`,`
 width: 200px;
 color: inherit;
 `,[e(`progress-text`,`
 font-weight: var(--n-font-weight-circle);
 color: var(--n-text-color-circle);
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 display: flex;
 align-items: center;
 justify-content: center;
 transition: color .3s var(--n-bezier);
 `)]),e(`progress-content`,{position:`relative`}),e(`progress-graph`,{position:`relative`},[e(`progress-graph-circle`,[i(`svg`,{verticalAlign:`bottom`}),e(`progress-graph-circle-fill`,`
 stroke: var(--n-fill-color);
 transition:
 opacity .3s var(--n-bezier),
 stroke .3s var(--n-bezier),
 stroke-dasharray .3s var(--n-bezier);
 `,[n(`empty`,{opacity:0})]),e(`progress-graph-circle-rail`,`
 transition: stroke .3s var(--n-bezier);
 overflow: hidden;
 stroke: var(--n-rail-color);
 `)]),e(`progress-graph-line`,[n(`indicator-inside`,[e(`progress-graph-line-rail`,`
 height: 16px;
 line-height: 16px;
 border-radius: 10px;
 `,[e(`progress-graph-line-fill`,`
 height: inherit;
 border-radius: 10px;
 `),e(`progress-graph-line-indicator`,`
 background: #0000;
 white-space: nowrap;
 text-align: right;
 margin-left: 14px;
 margin-right: 14px;
 height: inherit;
 font-size: 12px;
 color: var(--n-text-color-line-inner);
 transition: color .3s var(--n-bezier);
 `)])]),n(`indicator-inside-label`,`
 height: 16px;
 display: flex;
 align-items: center;
 `,[e(`progress-graph-line-rail`,`
 flex: 1;
 transition: background-color .3s var(--n-bezier);
 `),e(`progress-graph-line-indicator`,`
 background: var(--n-fill-color);
 font-size: 12px;
 transform: translateZ(0);
 display: flex;
 vertical-align: middle;
 height: 16px;
 line-height: 16px;
 padding: 0 10px;
 border-radius: 10px;
 position: absolute;
 white-space: nowrap;
 color: var(--n-text-color-line-inner);
 transition:
 right .2s var(--n-bezier),
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 `)]),e(`progress-graph-line-rail`,`
 position: relative;
 overflow: hidden;
 height: var(--n-rail-height);
 border-radius: 5px;
 background-color: var(--n-rail-color);
 transition: background-color .3s var(--n-bezier);
 `,[e(`progress-graph-line-fill`,`
 background: var(--n-fill-color);
 position: relative;
 border-radius: 5px;
 height: inherit;
 width: 100%;
 max-width: 0%;
 transition:
 background-color .3s var(--n-bezier),
 max-width .2s var(--n-bezier);
 `,[n(`processing`,[i(`&::after`,`
 content: "";
 background-image: var(--n-line-bg-processing);
 animation: progress-processing-animation 2s var(--n-bezier) infinite;
 `)])])])])])]),i(`@keyframes progress-processing-animation`,`
 0% {
 position: absolute;
 left: 0;
 top: 0;
 bottom: 0;
 right: 100%;
 opacity: 1;
 }
 66% {
 position: absolute;
 left: 0;
 top: 0;
 bottom: 0;
 right: 0;
 opacity: 0;
 }
 100% {
 position: absolute;
 left: 0;
 top: 0;
 bottom: 0;
 right: 0;
 opacity: 0;
 }
 `)]),G=[`aria-valuenow`,`role`],K={...t.props,processing:Boolean,type:{type:String,default:`line`},gapDegree:Number,gapOffsetDegree:Number,status:{type:String,default:`default`},railColor:[String,Array],railStyle:[String,Array],color:[String,Array,Object],viewBoxWidth:{type:Number,default:100},strokeWidth:{type:Number,default:7},percentage:[Number,Array],unit:{type:String,default:`%`},showIndicator:{type:Boolean,default:!0},indicatorPosition:{type:String,default:`outside`},indicatorPlacement:{type:String,default:`outside`},indicatorTextColor:String,circleGap:{type:Number,default:1},height:Number,borderRadius:[String,Number],fillBorderRadius:[String,Number],offsetDegree:Number},q=f({name:`Progress`,props:K,setup(e){let n=h(()=>e.indicatorPlacement||e.indicatorPosition),r=h(()=>{if(e.gapDegree||e.gapDegree===0)return e.gapDegree;if(e.type===`dashboard`)return 75}),{mergedClsPrefixRef:i,inlineThemeDisabled:o}=l(e),c=t(`Progress`,`-progress`,W,T,e,i),u=h(()=>{let{status:t}=e,{common:{cubicBezierEaseInOut:n},self:{fontSize:r,fontSizeCircle:i,railColor:a,railHeight:o,iconSizeCircle:l,iconSizeLine:u,textColorCircle:d,textColorLineInner:f,textColorLineOuter:p,lineBgProcessing:m,fontWeightCircle:h,[s(`iconColor`,t)]:g,[s(`fillColor`,t)]:_}}=c.value;return{"--n-bezier":n,"--n-fill-color":_,"--n-font-size":r,"--n-font-size-circle":i,"--n-font-weight-circle":h,"--n-icon-color":g,"--n-icon-size-circle":l,"--n-icon-size-line":u,"--n-line-bg-processing":m,"--n-rail-color":a,"--n-rail-height":o,"--n-text-color-circle":d,"--n-text-color-line-inner":f,"--n-text-color-line-outer":p}}),d=o?a(`progress`,h(()=>e.status[0]),u,e):void 0;return{mergedClsPrefix:i,mergedIndicatorPlacement:n,gapDeg:r,cssVars:o?void 0:u,themeClass:d?.themeClass,onRender:d?.onRender}},render(){let{type:e,cssVars:t,indicatorTextColor:n,showIndicator:i,status:a,railColor:o,railStyle:s,color:l,percentage:d,viewBoxWidth:f,strokeWidth:h,mergedIndicatorPlacement:g,unit:b,borderRadius:x,fillBorderRadius:S,height:C,processing:w,circleGap:T,mergedClsPrefix:E,gapDeg:D,gapOffsetDegree:O,themeClass:k,$slots:A,onRender:j}=this;return j?.(),p(),_(`div`,{class:r([k,`${E}-progress`,`${E}-progress--${e}`,`${E}-progress--${a}`]),style:m(t),"aria-valuemax":100,"aria-valuemin":0,"aria-valuenow":d,role:e===`circle`||e===`line`||e===`dashboard`?`progressbar`:`none`},[e===`circle`||e===`dashboard`?(p(),y(N,{key:0,clsPrefix:E,status:a,showIndicator:i,indicatorTextColor:n,railColor:o,fillColor:l,railStyle:s,offsetDegree:this.offsetDegree,percentage:d,viewBoxWidth:f,strokeWidth:h,gapDegree:D===void 0?e===`dashboard`?75:0:D,gapOffsetDegree:O,unit:b},c(A),1032,[`clsPrefix`,`status`,`showIndicator`,`indicatorTextColor`,`railColor`,`fillColor`,`railStyle`,`offsetDegree`,`percentage`,`viewBoxWidth`,`strokeWidth`,`gapDegree`,`gapOffsetDegree`,`unit`])):(p(),_(v,{key:1},[e===`line`?(p(),y(F,{key:0,clsPrefix:E,status:a,showIndicator:i,indicatorTextColor:n,railColor:o,fillColor:l,railStyle:s,percentage:d,processing:w,indicatorPlacement:g,unit:b,fillBorderRadius:S,railBorderRadius:x,height:C},c(A),1032,[`clsPrefix`,`status`,`showIndicator`,`indicatorTextColor`,`railColor`,`fillColor`,`railStyle`,`percentage`,`processing`,`indicatorPlacement`,`unit`,`fillBorderRadius`,`railBorderRadius`,`height`])):(p(),_(v,{key:1},[e===`multiple-circle`?(p(),y(U,{key:0,clsPrefix:E,strokeWidth:h,railColor:o,fillColor:l,railStyle:s,viewBoxWidth:f,percentage:d,showIndicator:i,circleGap:T},c(A),1032,[`clsPrefix`,`strokeWidth`,`railColor`,`fillColor`,`railStyle`,`viewBoxWidth`,`percentage`,`showIndicator`,`circleGap`])):u(()=>null)],64))],64))],14,G)}});export{q as t};