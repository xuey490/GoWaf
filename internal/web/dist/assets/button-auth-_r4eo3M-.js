import{S as e,at as t,c as n,ct as r,g as i,it as a,l as o,n as s,r as c,st as l,u,ut as d,w as f,y as p}from"./resolve-slot-D0ESJhJD.js";import{t as m}from"./use-rtl-B41EZXdW.js";import{f as h}from"./light-D4diJaDS.js";import{I as g,b as _,t as v,tt as y}from"./user-CuXVhm31.js";import{Et as b,Gt as x,M as S,R as C,S as w,St as T,V as E,W as ee,Wt as D,_t as O,at as k,b as A,d as j,f as M,h as N,i as te,m as ne,ot as P,p as F,q as I,rt as L,y as R}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{n as re,t as z}from"./runtime-dom.esm-bundler-B3Pl3ttw.js";import{t as B}from"./use-style-CXm9Jcwk.js";import{t as V}from"./color-to-class-G4jmrDIg.js";import{t as H}from"./Tag-CfwO544n.js";import{d as U,o as W,s as G,t as K}from"./Button-DUNxlS0h.js";import{t as q}from"./Alert-DfEI47PE.js";import{s as J}from"./Select-3eV9jO8m.js";import{t as Y}from"./Card-D_p3JLbh.js";import{t as X}from"./Dropdown-FOBRA_8j.js";import{t as ie}from"./Switch-UvvEAMeb.js";import{t as ae}from"./DownOutlined-_DqcU22h.js";import{t as oe}from"./LockOutlined-D8LOtJz_.js";import{t as se}from"./art-table-CypTBawk.js";import{t as ce}from"./art-table-column-syKm23yt.js";import{t as le}from"./useAuth-C4nClyB1.js";var Z=w({name:`SlotMachineNumber`,props:{clsPrefix:{type:String,required:!0},value:{type:[Number,String],required:!0},oldOriginalNumber:{type:Number,default:void 0},newOriginalNumber:{type:Number,default:void 0}},setup(e){let t=O(null),n=O(e.value),r=O(e.value),a=O(`up`),o=O(!1),s=j(()=>o.value?`${e.clsPrefix}-base-slot-machine-current-number--${a.value}-scroll`:null),c=j(()=>o.value?`${e.clsPrefix}-base-slot-machine-old-number--${a.value}-scroll`:null);L(T(e,`value`),(e,t)=>{n.value=t,r.value=e,S(l)});function l(){let t=e.newOriginalNumber,n=e.oldOriginalNumber;n!==void 0&&t!==void 0&&(t>n?u(`up`):n>t&&u(`down`))}function u(e){a.value=e,o.value=!1,S(()=>{t.value?.offsetWidth,o.value=!0})}return()=>{let{clsPrefix:a}=e;return E(),N(`span`,{ref:t,class:i(`${a}-base-slot-machine-number`)},[n.value===null?p(()=>null):(E(),N(`span`,{key:0,class:i([`${a}-base-slot-machine-old-number ${a}-base-slot-machine-old-number--top`,c.value])},[p(()=>n.value)],2)),M(`span`,{class:i([`${a}-base-slot-machine-current-number`,s.value])},[M(`span`,{ref:`numberWrapper`,class:i([`${a}-base-slot-machine-current-number__inner`,typeof e.value!=`number`&&`${a}-base-slot-machine-current-number__inner--not-number`])},[p(()=>r.value)],2)],2),n.value===null?p(()=>null):(E(),N(`span`,{key:2,class:i([`${a}-base-slot-machine-old-number ${a}-base-slot-machine-old-number--bottom`,c.value])},[p(()=>n.value)],2))],2)}}}),{cubicBezierEaseOut:Q}=e;function ue({duration:e=`.2s`}={}){return[a(`&.fade-up-width-expand-transition-leave-active`,{transition:`
 opacity ${e} ${Q},
 max-width ${e} ${Q},
 transform ${e} ${Q}
 `}),a(`&.fade-up-width-expand-transition-enter-active`,{transition:`
 opacity ${e} ${Q},
 max-width ${e} ${Q},
 transform ${e} ${Q}
 `}),a(`&.fade-up-width-expand-transition-enter-to`,{opacity:1,transform:`translateX(0) translateY(0)`}),a(`&.fade-up-width-expand-transition-enter-from`,{maxWidth:`0 !important`,opacity:0,transform:`translateY(60%)`}),a(`&.fade-up-width-expand-transition-leave-from`,{opacity:1,transform:`translateY(0)`}),a(`&.fade-up-width-expand-transition-leave-to`,{maxWidth:`0 !important`,opacity:0,transform:`translateY(60%)`})]}var de=a([a(`@keyframes n-base-slot-machine-fade-up-in`,`
 from {
 transform: translateY(60%);
 opacity: 0;
 }
 to {
 transform: translateY(0);
 opacity: 1;
 }
 `),a(`@keyframes n-base-slot-machine-fade-down-in`,`
 from {
 transform: translateY(-60%);
 opacity: 0;
 }
 to {
 transform: translateY(0);
 opacity: 1;
 }
 `),a(`@keyframes n-base-slot-machine-fade-up-out`,`
 from {
 transform: translateY(0%);
 opacity: 1;
 }
 to {
 transform: translateY(-60%);
 opacity: 0;
 }
 `),a(`@keyframes n-base-slot-machine-fade-down-out`,`
 from {
 transform: translateY(0%);
 opacity: 1;
 }
 to {
 transform: translateY(60%);
 opacity: 0;
 }
 `),t(`base-slot-machine`,`
 overflow: hidden;
 white-space: nowrap;
 display: inline-block;
 height: 18px;
 line-height: 18px;
 `,[t(`base-slot-machine-number`,`
 display: inline-block;
 position: relative;
 height: 18px;
 width: .6em;
 max-width: .6em;
 `,[ue({duration:`.2s`}),G({duration:`.2s`,delay:`0s`}),t(`base-slot-machine-old-number`,`
 display: inline-block;
 opacity: 0;
 position: absolute;
 left: 0;
 right: 0;
 `,[r(`top`,{transform:`translateY(-100%)`}),r(`bottom`,{transform:`translateY(100%)`}),r(`down-scroll`,{animation:`n-base-slot-machine-fade-down-out .2s cubic-bezier(0, 0, .2, 1)`,animationIterationCount:1}),r(`up-scroll`,{animation:`n-base-slot-machine-fade-up-out .2s cubic-bezier(0, 0, .2, 1)`,animationIterationCount:1})]),t(`base-slot-machine-current-number`,`
 display: inline-block;
 position: absolute;
 left: 0;
 top: 0;
 bottom: 0;
 right: 0;
 opacity: 1;
 transform: translateY(0);
 width: .6em;
 `,[r(`down-scroll`,{animation:`n-base-slot-machine-fade-down-in .2s cubic-bezier(0, 0, .2, 1)`,animationIterationCount:1}),r(`up-scroll`,{animation:`n-base-slot-machine-fade-up-in .2s cubic-bezier(0, 0, .2, 1)`,animationIterationCount:1}),l(`inner`,`
 display: inline-block;
 position: absolute;
 right: 0;
 top: 0;
 width: .6em;
 `,[r(`not-number`,`
 right: unset;
 left: 0;
 `)])])])])]),fe=w({name:`BaseSlotMachine`,props:{clsPrefix:{type:String,required:!0},value:{type:[Number,String],default:0},max:{type:Number,default:void 0},appeared:{type:Boolean,required:!0}},setup(e){B(`-base-slot-machine`,de,T(e,`clsPrefix`));let t=O(),n=O(),r=j(()=>{if(typeof e.value==`string`)return[];if(e.value<1)return[0];let t=[],n=e.value;for(e.max!==void 0&&(n=Math.min(e.max,n));n>=1;)t.push(n%10),n/=10,n=Math.floor(n);return t.reverse(),t});return L(T(e,`value`),(e,r)=>{typeof e==`string`?(n.value=void 0,t.value=void 0):typeof r==`string`?(n.value=e,t.value=void 0):(n.value=e,t.value=r)}),()=>{let{value:a,clsPrefix:o}=e;return typeof a==`number`?(E(),N(`span`,{key:1,class:i(`${o}-base-slot-machine`)},[A(re,{name:`fade-up-width-expand-transition`,tag:`span`},{default:()=>r.value.map((e,i)=>(E(),F(Z,{clsPrefix:o,key:r.value.length-i-1,oldOriginalNumber:t.value,newOriginalNumber:n.value,value:e},null,8,[`clsPrefix`,`oldOriginalNumber`,`newOriginalNumber`,`value`])))},1024),A(U,{key:`+`,width:!0},{default:()=>e.max!==void 0&&e.max<a?(E(),F(Z,{key:2,clsPrefix:o,value:`+`},null,8,[`clsPrefix`])):null},1024)],2)):(E(),N(`span`,{key:3,class:i(`${o}-base-slot-machine`)},[p(()=>a)],2))}}});function pe(e){let{errorColor:t,infoColor:n,successColor:r,warningColor:i,fontFamily:a}=e;return{color:t,colorInfo:n,colorSuccess:r,colorError:t,colorWarning:i,fontSize:`12px`,fontFamily:a}}var me={name:`Badge`,common:u,self:pe},he=a([a(`@keyframes badge-wave-spread`,{from:{boxShadow:`0 0 0.5px 0px var(--n-ripple-color)`,opacity:.6},to:{boxShadow:`0 0 0.5px 4.5px var(--n-ripple-color)`,opacity:0}}),t(`badge`,`
 display: inline-flex;
 position: relative;
 vertical-align: middle;
 font-family: var(--n-font-family);
 `,[r(`as-is`,[t(`badge-sup`,{position:`static`,transform:`translateX(0)`},[h({transformOrigin:`left bottom`,originalTransform:`translateX(0)`})])]),r(`dot`,[t(`badge-sup`,`
 height: 8px;
 width: 8px;
 padding: 0;
 min-width: 8px;
 left: 100%;
 bottom: calc(100% - 4px);
 `,[a(`::before`,`border-radius: 4px;`)])]),t(`badge-sup`,`
 background: var(--n-color);
 transition:
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
 color: #FFF;
 position: absolute;
 height: 18px;
 line-height: 18px;
 border-radius: 9px;
 padding: 0 6px;
 text-align: center;
 font-size: var(--n-font-size);
 transform: translateX(-50%);
 left: 100%;
 bottom: calc(100% - 9px);
 font-variant-numeric: tabular-nums;
 z-index: 2;
 display: flex;
 align-items: center;
 `,[h({transformOrigin:`left bottom`,originalTransform:`translateX(-50%)`}),t(`base-wave`,{zIndex:1,animationDuration:`2s`,animationIterationCount:`infinite`,animationDelay:`1s`,animationTimingFunction:`var(--n-ripple-bezier)`,animationName:`badge-wave-spread`}),a(`&::before`,`
 opacity: 0;
 transform: scale(1);
 border-radius: 9px;
 content: "";
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 `)])])]),ge=[`title`],_e={...n.props,value:[String,Number],max:Number,dot:Boolean,type:{type:String,default:`default`},show:{type:Boolean,default:!0},showZero:Boolean,processing:Boolean,color:String,offset:Array},ve=w({name:`Badge`,props:_e,setup(e,{slots:t}){let{mergedClsPrefixRef:r,inlineThemeDisabled:i,mergedRtlRef:a}=f(e),c=n(`Badge`,`-badge`,he,me,e,r),l=O(!1),u=()=>{l.value=!0},p=()=>{l.value=!1},h=j(()=>e.show&&(e.dot||e.value!==void 0&&!(!e.showZero&&Number(e.value)<=0)||!s(t.value)));C(()=>{h.value&&(l.value=!0)});let g=m(`Badge`,a,r),_=j(()=>{let{type:t,color:n}=e,{common:{cubicBezierEaseInOut:r,cubicBezierEaseOut:i},self:{[d(`color`,t)]:a,fontFamily:o,fontSize:s}}=c.value;return{"--n-font-size":s,"--n-font-family":o,"--n-color":n||a,"--n-ripple-color":n||a,"--n-bezier":r,"--n-ripple-bezier":i}}),v=i?o(`badge`,j(()=>{let t=``,{type:n,color:r}=e;return n&&(t+=n[0]),r&&(t+=V(r)),t}),_,e):void 0,y=j(()=>{let{offset:t}=e;if(!t)return;let[n,r]=t,i=typeof n==`number`?`${n}px`:n,a=typeof r==`number`?`${r}px`:r;return{transform:`translate(calc(${g?.value?`50%`:`-50%`} + ${i}), ${a})`}});return{rtlEnabled:g,mergedClsPrefix:r,appeared:l,showBadge:h,handleAfterEnter:u,handleAfterLeave:p,cssVars:i?void 0:_,themeClass:v?.themeClass,onRender:v?.onRender,offsetStyle:y}},render(){let{mergedClsPrefix:e,onRender:t,themeClass:n,$slots:r}=this;t?.();let a=r.default?.();return E(),N(`div`,{class:i([`${e}-badge`,this.rtlEnabled&&`${e}-badge--rtl`,n,{[`${e}-badge--dot`]:this.dot,[`${e}-badge--as-is`]:!a}]),style:D(this.cssVars)},[p(()=>a),(E(),F(z,{name:`fade-in-scale-up-transition`,onAfterEnter:this.handleAfterEnter,onAfterLeave:this.handleAfterLeave},{default:()=>this.showBadge?(E(),N(`sup`,{key:1,class:i(`${e}-badge-sup`),title:J(this.value),style:D(this.offsetStyle)},[p(()=>c(r.value,()=>[this.dot?null:(E(),F(fe,{key:2,clsPrefix:e,appeared:this.appeared,max:this.max,value:this.value},null,8,[`clsPrefix`,`appeared`,`max`,`value`]))])),this.processing?(E(),F(W,{key:0,clsPrefix:e},null,8,[`clsPrefix`])):p(()=>null)],14,ge)):null},1032,[`onAfterEnter`,`onAfterLeave`]))],6)}}),ye={class:`w-full py-2`},be={class:`mb-6`},xe={class:`m-0 mb-2 text-xl font-medium`},Se={class:`mb-6`},Ce={class:`flex items-start mb-4 last:mb-0`},we={class:`flex items-start mb-4 last:mb-0`},Te={class:`flex flex-wrap gap-2`},Ee={key:0,class:`italic text-red-500`},De={class:`mb-6 last:mb-0`},Oe={class:`grid grid-cols-[repeat(auto-fit,minmax(280px,1fr))] gap-5`},ke={class:`flex flex-col gap-2`},Ae={class:`flex flex-col gap-2`},je={class:`flex flex-col gap-2`},Me={class:`mb-6 last:mb-0`},Ne={class:`flex-cb font-semibold`},Pe={class:`mb-5`},Fe={class:`max-h-30 p-3 overflow-y-auto font-mono text-xs break-all whitespace-pre-wrap bg-g-200 border-full-d rounded`},Ie={class:`grid grid-cols-[repeat(auto-fit,minmax(280px,1fr))] gap-5`},Le={class:`flex flex-col gap-2`},Re={class:`flex flex-col gap-2`},ze={class:`flex flex-col gap-2`},Be={class:`flex flex-col gap-2`},Ve={class:`mt-4`},He={class:`mb-6 last:mb-0`},Ue={class:`flex-cb font-semibold`},We={class:`mb-5`},Ge={class:`max-h-30 p-3 overflow-y-auto font-mono text-xs break-all whitespace-pre-wrap bg-g-200 border-full-d rounded`},Ke={class:`grid grid-cols-[repeat(auto-fit,minmax(280px,1fr))] gap-5`},qe={class:`flex flex-col gap-2`},Je={class:`flex flex-col gap-2`},$={class:`flex flex-col gap-2`},Ye={class:`mb-6 last:mb-0`},Xe={class:`flex-cb font-semibold`},Ze={class:`grid grid-cols-[repeat(auto-fit,minmax(280px,1fr))] gap-5`},Qe={class:`flex flex-col gap-2`},$e={class:`flex flex-col gap-2`},et={key:0,class:`flex gap-2 p-3 bg-g-200 border-full-d rounded`},tt={key:1,class:`flex-c gap-2 p-3 text-g-500 bg-g-200 border border-dashed border-g-400 rounded`},nt={class:`flex flex-col gap-2`},rt={class:`flex flex-wrap gap-3`},it={class:`mb-6 last:mb-0`},at={class:`mt-4`},ot=w({name:`PermissionButtonAuth`,__name:`index`,setup(e){let{hasAuth:t}=le(),{isFrontendMode:n}=g(),r=v(),i=y(),a=O(!1),o=j(()=>r.info?.roles?.[0]||``),s=j(()=>r.info?.buttons||[]),c=j(()=>r.info?.buttons||[]),l=j(()=>Array.isArray(i.meta.authList)?i.meta.authList:[]),u=j(()=>t(`edit`)||t(`delete`)||t(`export`)),d=j(()=>[{label:`批量编辑`,key:`batchEdit`,disabled:!t(`edit`)},{label:`批量删除`,key:`batchDelete`,disabled:!t(`delete`)},{label:`批量导出`,key:`batchExport`,disabled:!t(`export`)}]),f=j(()=>[{feature:`权限来源`,frontend:`用户状态中的 buttons 字段`,backend:`路由配置中的 meta.authList`},{feature:`v-auth 指令`,frontend:`不可用（前端模式下指令无效）`,backend:`可用（根据路由权限配置检查）`},{feature:`hasAuth 方法`,frontend:`可用（检查 buttons 数组）`,backend:`可用（检查 meta.authList）`},{feature:`权限管理`,frontend:`完全由前端控制，灵活度高`,backend:`后端统一管理，安全性更高`},{feature:`适用场景`,frontend:`快速原型、简单应用`,backend:`企业级应用、复杂权限体系`}]),p=e=>({R_SUPER:`warning`,R_ADMIN:`info`,R_USER:`success`})[e]||`default`,m=e=>({R_SUPER:`超级管理员`,R_ADMIN:`管理员`,R_USER:`普通用户`})[e]||`未知角色`,h=()=>{t(`publish`)?_.success(`发布成功！`):_.warning(`您没有发布权限`)},S=e=>{C(String(e))},C=e=>{let n={batchEdit:`批量编辑`,batchDelete:`批量删除`,batchExport:`批量导出`},r=e.replace(`batch`,``).toLowerCase();t(r)?_.success(`${n[e]}操作执行成功！`):_.warning(`您没有${n[e]}权限`)};return(e,r)=>{let i=H,g=Y,_=K,v=q,y=X,C=ie,w=ve,T=ce,D=se,O=I(`roles`),j=I(`auth`);return E(),N(`div`,ye,[M(`div`,be,[M(`h2`,xe,x(e.$t(`menus.examples.permission.buttonAuth`)),1),r[1]||(r[1]=M(`p`,{class:`m-0 text-sm leading-[1.6] text-g-700`},` 此页面演示按钮级别的权限控制，不同用户身份下可见的按钮会有所不同。权限控制基于用户角色和权限码实现。 `,-1))]),M(`div`,Se,[A(g,{class:`art-card-xs`},{header:k(()=>[...r[2]||(r[2]=[M(`div`,{class:`flex-cb font-semibold`},[M(`span`,{class:`flex-1`},`当前用户权限信息`)],-1)])]),default:k(()=>[M(`div`,null,[M(`div`,Ce,[r[3]||(r[3]=M(`span`,{class:`min-w-20 font-semibold`},`用户角色：`,-1)),A(i,{type:p(o.value)},{default:k(()=>[R(x(m(o.value)),1)]),_:1},8,[`type`])]),M(`div`,we,[r[4]||(r[4]=M(`span`,{class:`min-w-20 font-semibold`},`权限码：`,-1)),M(`div`,Te,[(E(!0),N(te,null,ee(s.value,e=>(E(),F(i,{key:e,class:`m-0`},{default:k(()=>[R(x(e),1)]),_:2},1024))),128)),s.value.length?ne(``,!0):(E(),N(`span`,Ee,`无权限码`))])])])]),_:1})]),M(`div`,De,[A(g,{class:`art-card-xs`},{header:k(()=>[...r[5]||(r[5]=[M(`div`,{class:`flex-cb font-semibold`},[M(`span`,{class:`flex-1`},`基于角色的权限控制（v-roles 指令）`)],-1)])]),default:k(()=>[M(`div`,null,[r[12]||(r[12]=M(`p`,{class:`p-3 m-0 mb-5 text-sm leading-[1.6] text-g-700 bg-g-200 rounded`},[R(` 使用 `),M(`code`,{class:`px- 1.5 py-0.5 font-mono text-xs text-theme bg-theme/10 rounded`},`v-roles`),R(` 指令控制按钮显示，只有拥有指定角色的用户才能看到对应按钮。 `)],-1)),M(`div`,Oe,[M(`div`,ke,[P((E(),F(_,{type:`primary`},{default:k(()=>[...r[6]||(r[6]=[R(` 超级管理员可见 `,-1)])]),_:1})),[[O,`R_SUPER`]]),r[7]||(r[7]=M(`div`,{class:`text-xs`},[M(`code`,{class:`block p-1 px-2 mb-1 font-mono bg-g-200 border-full-d rounded`},`v-roles="'R_SUPER'"`),M(`span`,{class:`text-g-700`},`只有超级管理员可见`)],-1))]),M(`div`,Ae,[P((E(),F(_,{type:`error`},{default:k(()=>[...r[8]||(r[8]=[R(` 管理员可见 `,-1)])]),_:1})),[[O,[`R_SUPER`,`R_ADMIN`]]]),r[9]||(r[9]=M(`div`,{class:`text-xs`},[M(`code`,{class:`block p-1 px-2 mb-1 font-mono bg-g-200 border-full-d rounded`},`v-roles="['R_SUPER', 'R_ADMIN']"`),M(`span`,{class:`text-g-700`},`超级管理员和管理员可见`)],-1))]),M(`div`,je,[P((E(),F(_,{type:`primary`},{default:k(()=>[...r[10]||(r[10]=[R(` 所有用户可见 `,-1)])]),_:1})),[[O,[`R_SUPER`,`R_ADMIN`,`R_USER`]]]),r[11]||(r[11]=M(`div`,{class:`text-xs`},[M(`code`,{class:`block p-1 px-2 mb-1 font-mono bg-g-200 border-full-d rounded`},`v-roles="['R_SUPER', 'R_ADMIN', 'R_USER']"`),M(`span`,{class:`text-g-700`},`所有已登录用户可见`)],-1))])])])]),_:1})]),M(`div`,Me,[A(g,{class:`art-card-xs`},{header:k(()=>[M(`div`,Ne,[r[15]||(r[15]=M(`span`,{class:`flex-1`},`后端模式权限控制（v-auth 指令）`,-1)),b(n)?(E(),F(i,{key:1},{default:k(()=>[...r[14]||(r[14]=[R(`非当前模式`,-1)])]),_:1})):(E(),F(i,{key:0,type:`success`},{default:k(()=>[...r[13]||(r[13]=[R(`当前模式`,-1)])]),_:1}))])]),default:k(()=>[M(`div`,null,[r[25]||(r[25]=M(`p`,{class:`p-3 m-0 mb-5 text-sm leading-[1.6] text-g-700 bg-g-200 rounded`},[M(`code`,{class:`px- 1.5 py-0.5 font-mono text-xs text-theme bg-theme/10 rounded`},`v-auth`),R(` 指令在后端模式下根据当前路由的 `),M(`code`,{class:`px- 1.5 py-0.5 font-mono text-xs text-theme bg-theme/10 rounded`},`meta.authList`),R(` 配置检查权限。 权限列表来源于路由元数据，适用于权限配置由后端统一管理的场景。 `)],-1)),M(`div`,Pe,[r[16]||(r[16]=M(`h4`,{class:`m-0 mb-2 text-sm font-semibold`},` 当前路由权限配置： `,-1)),M(`div`,Fe,[M(`code`,null,x(JSON.stringify(l.value,null,2)),1)])]),M(`div`,Ie,[M(`div`,Le,[P((E(),F(_,{type:`primary`},{default:k(()=>[...r[17]||(r[17]=[R(` 新增 `,-1)])]),_:1})),[[j,`add`]]),r[18]||(r[18]=M(`div`,{class:`text-xs`},[M(`code`,{class:`block p-1 px-2 mb-1 font-mono bg-g-200 border-full-d rounded`},`v-auth="'add'"`),M(`span`,{class:`text-g-700`},`检查路由 meta.authList 中是否存在 authMark: 'add'`)],-1))]),M(`div`,Re,[P((E(),F(_,{type:`error`},{default:k(()=>[...r[19]||(r[19]=[R(` 编辑 `,-1)])]),_:1})),[[j,`edit`]]),r[20]||(r[20]=M(`div`,{class:`text-xs`},[M(`code`,{class:`block p-1 px-2 mb-1 font-mono bg-g-200 border-full-d rounded`},`v-auth="'edit'"`),M(`span`,{class:`text-g-700`},`检查路由 meta.authList 中是否存在 authMark: 'edit'`)],-1))]),M(`div`,ze,[P((E(),F(_,{type:`error`},{default:k(()=>[...r[21]||(r[21]=[R(` 删除 `,-1)])]),_:1})),[[j,`delete`]]),r[22]||(r[22]=M(`div`,{class:`text-xs`},[M(`code`,{class:`block p-1 px-2 mb-1 font-mono bg-g-200 border-full-d rounded`},`v-auth="'delete'"`),M(`span`,{class:`text-g-700`},`检查路由 meta.authList 中是否存在 authMark: 'delete'`)],-1))]),M(`div`,Be,[P((E(),F(_,null,{default:k(()=>[...r[23]||(r[23]=[R(` 导出 `,-1)])]),_:1})),[[j,`export`]]),r[24]||(r[24]=M(`div`,{class:`text-xs`},[M(`code`,{class:`block p-1 px-2 mb-1 font-mono bg-g-200 border-full-d rounded`},`v-auth="'export'"`),M(`span`,{class:`text-g-700`},`检查路由 meta.authList 中是否存在 authMark: 'export'`)],-1))])]),M(`div`,Ve,[A(v,{type:b(n)?`warning`:`success`,title:b(n)?`注意：当前为前端模式，v-auth 指令在后端模式下才生效`:`当前为后端模式，v-auth 指令正常工作`},null,8,[`type`,`title`])])])]),_:1})]),M(`div`,He,[A(g,{class:`art-card-xs`},{header:k(()=>[M(`div`,Ue,[r[28]||(r[28]=M(`span`,{class:`flex-1`},`前端模式权限控制（hasAuth 方法）`,-1)),b(n)?(E(),F(i,{key:0,type:`success`},{default:k(()=>[...r[26]||(r[26]=[R(`当前模式`,-1)])]),_:1})):(E(),F(i,{key:1},{default:k(()=>[...r[27]||(r[27]=[R(`非当前模式`,-1)])]),_:1}))])]),default:k(()=>[M(`div`,null,[r[36]||(r[36]=M(`p`,{class:`p-3 m-0 mb-5 text-sm leading-[1.6] text-g-700 bg-g-200 rounded`},[M(`code`,{class:`px- 1.5 py-0.5 font-mono text-xs text-theme bg-theme/10 rounded`},`hasAuth`),R(` 方法在前端模式下根据用户信息中的 `),M(`code`,{class:`px- 1.5 py-0.5 font-mono text-xs text-theme bg-theme/10 rounded`},`buttons`),R(` 字段检查权限。 权限列表存储在用户状态中，适用于权限完全由前端管理的场景。 `)],-1)),M(`div`,We,[r[29]||(r[29]=M(`h4`,{class:`m-0 mb-2 text-sm font-semibold`},`当前用户权限列表（buttons 字段）：`,-1)),M(`div`,Ge,[M(`code`,null,x(JSON.stringify(c.value,null,2)),1)])]),M(`div`,Ke,[M(`div`,qe,[b(t)(`view`)?(E(),F(_,{key:0,type:`primary`},{default:k(()=>[...r[30]||(r[30]=[R(` 查看详情 `,-1)])]),_:1})):(E(),F(_,{key:1,disabled:``},{default:k(()=>[...r[31]||(r[31]=[R(` 无查看权限 `,-1)])]),_:1})),r[32]||(r[32]=M(`div`,{class:`text-xs`},[M(`code`,{class:`block p-1 px-2 mb-1 font-mono bg-g-200 border-full-d rounded`},`v-if="hasAuth('view')"`),M(`span`,{class:`text-g-700`},`检查用户 buttons 数组是否包含 'view'`)],-1))]),M(`div`,Je,[A(_,{disabled:!b(t)(`publish`),type:b(t)(`publish`)?`primary`:`default`,onClick:h},{default:k(()=>[R(x(b(t)(`publish`)?`发布`:`无发布权限`),1)]),_:1},8,[`disabled`,`type`]),r[33]||(r[33]=M(`div`,{class:`text-xs`},[M(`code`,{class:`block p-1 px-2 mb-1 font-mono bg-g-200 border-full-d rounded`},`:disabled="!hasAuth('publish')"`),M(`span`,{class:`text-g-700`},`检查用户 buttons 数组是否包含 'publish'`)],-1))]),M(`div`,$,[A(y,{disabled:!u.value,options:d.value,onSelect:S},{default:k(()=>[A(_,{type:u.value?`error`:`default`},{default:k(()=>[r[34]||(r[34]=R(` 批量操作 `,-1)),A(b(ae))]),_:1},8,[`type`])]),_:1},8,[`disabled`,`options`]),r[35]||(r[35]=M(`div`,{class:`text-xs`},[M(`code`,{class:`block p-1 px-2 mb-1 font-mono bg-g-200 border-full-d rounded`},`computed(() => hasAuth('edit') || hasAuth('delete'))`),M(`span`,{class:`text-g-700`},`复合权限检查，检查多个权限的并集`)],-1))])])])]),_:1})]),M(`div`,Ye,[A(g,{class:`art-card-xs`},{header:k(()=>[M(`div`,Xe,[r[39]||(r[39]=M(`span`,{class:`flex-1`},`后端模式权限控制（hasAuth 方法）`,-1)),b(n)?(E(),F(i,{key:1},{default:k(()=>[...r[38]||(r[38]=[R(`非当前模式`,-1)])]),_:1})):(E(),F(i,{key:0,type:`success`},{default:k(()=>[...r[37]||(r[37]=[R(`当前模式`,-1)])]),_:1}))])]),default:k(()=>[M(`div`,null,[r[51]||(r[51]=M(`p`,{class:`p-3 m-0 mb-5 text-sm leading-[1.6] text-g-700 bg-g-200 rounded`},[M(`code`,{class:`px- 1.5 py-0.5 font-mono text-xs text-theme bg-theme/10 rounded`},`hasAuth`),R(` 方法在后端模式下根据当前路由的 `),M(`code`,{class:`px- 1.5 py-0.5 font-mono text-xs text-theme bg-theme/10 rounded`},`meta.authList`),R(` 配置检查权限。与 v-auth 指令使用相同的权限来源，但提供编程式的权限检查能力。 `)],-1)),M(`div`,Ze,[M(`div`,Qe,[A(C,{value:a.value,"onUpdate:value":r[0]||(r[0]=e=>a.value=e),disabled:!b(t)(`config`)},{checked:k(()=>[...r[40]||(r[40]=[R(`功能开启`,-1)])]),unchecked:k(()=>[...r[41]||(r[41]=[R(`功能关闭`,-1)])]),_:1},8,[`value`,`disabled`]),r[42]||(r[42]=M(`div`,{class:`text-xs`},[M(`code`,{class:`block p-1 px-2 mb-1 font-mono bg-g-200 border-full-d rounded`},`:disabled="!hasAuth('config')"`),M(`span`,{class:`text-g-700`},`检查路由权限控制功能开关`)],-1))]),M(`div`,$e,[b(t)(`manage`)?(E(),N(`div`,et,[A(_,{type:`primary`,size:`small`},{default:k(()=>[...r[43]||(r[43]=[R(`管理操作`,-1)])]),_:1}),A(_,{type:`error`,size:`small`},{default:k(()=>[...r[44]||(r[44]=[R(`高级设置`,-1)])]),_:1})])):(E(),N(`div`,tt,[A(b(oe)),r[45]||(r[45]=M(`span`,{class:`text-sm`},`需要管理权限才能查看此区域`,-1))])),r[46]||(r[46]=M(`div`,{class:`text-xs`},[M(`code`,{class:`block p-1 px-2 mb-1 font-mono bg-g-200 border-full-d rounded`},`v-if="hasAuth('manage')"`),M(`span`,{class:`text-g-700`},`基于后端路由权限的条件渲染`)],-1))]),M(`div`,nt,[M(`div`,rt,[A(w,{value:b(t)(`add`)?`✓`:`✗`,type:b(t)(`add`)?`success`:`error`},{default:k(()=>[A(_,{size:`small`},{default:k(()=>[...r[47]||(r[47]=[R(`新增权限`,-1)])]),_:1})]),_:1},8,[`value`,`type`]),A(w,{value:b(t)(`edit`)?`✓`:`✗`,type:b(t)(`edit`)?`success`:`error`},{default:k(()=>[A(_,{size:`small`},{default:k(()=>[...r[48]||(r[48]=[R(`编辑权限`,-1)])]),_:1})]),_:1},8,[`value`,`type`]),A(w,{value:b(t)(`delete`)?`✓`:`✗`,type:b(t)(`delete`)?`success`:`error`},{default:k(()=>[A(_,{size:`small`},{default:k(()=>[...r[49]||(r[49]=[R(`删除权限`,-1)])]),_:1})]),_:1},8,[`value`,`type`])]),r[50]||(r[50]=M(`div`,{class:`text-xs`},[M(`code`,{class:`block p-1 px-2 mb-1 font-mono bg-g-200 border-full-d rounded`},`hasAuth('permission')`),M(`span`,{class:`text-g-700`},`实时权限状态指示器`)],-1))])])])]),_:1})]),M(`div`,it,[A(g,{class:`art-card-xs`},{header:k(()=>[...r[52]||(r[52]=[M(`div`,{class:`flex-cb font-semibold`},[M(`span`,{class:`flex-1`},`权限控制模式对比`)],-1)])]),default:k(()=>[M(`div`,null,[M(`div`,at,[A(D,{data:f.value,border:``},{default:k(()=>[A(T,{"data-index":`feature`,title:`功能特性`,width:150}),A(T,{"data-index":`frontend`,title:`前端模式`}),A(T,{"data-index":`backend`,title:`后端模式`})]),_:1},8,[`data`])])])]),_:1})])])}}});export{ot as default};