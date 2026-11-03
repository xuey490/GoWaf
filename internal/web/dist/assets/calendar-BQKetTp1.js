import{i as e}from"./rolldown-runtime-COiEfhsA.js";import{at as t,c as n,ct as r,dt as i,ft as a,g as o,i as s,it as c,l,lt as u,st as d,w as f,y as p}from"./resolve-slot-D0ESJhJD.js";import{n as m,t as h}from"./use-rtl-B41EZXdW.js";import{n as g}from"./replaceable-CG1Qs4lR.js";import{Et as _,Gt as v,H as y,S as b,St as x,V as S,Vt as ee,W as C,Wt as w,_t as T,at as E,b as D,d as O,dt as k,f as A,h as j,i as M,m as te,p as N,y as P}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{s as F}from"./runtime-dom.esm-bundler-B3Pl3ttw.js";import{t as I}from"./use-style-CXm9Jcwk.js";import{t as L}from"./use-locale-JKfGS3H1.js";import{g as R,t as ne}from"./Modal-BgtV8YUs.js";import{t as re}from"./use-merged-state-KQOIh2IK.js";import{r as ie,t as z}from"./Button-DUNxlS0h.js";import{i as B,t as ae}from"./FormItem-BAj_XuOv.js";import{t as oe}from"./Input-ByrqO_2o.js";import{t as V}from"./ChevronLeft-Bb8WFTYP.js";import{t as se}from"./ChevronRight-CSGCWNSb.js";import{a as H,c as U,i as W,l as G,o as K,r as ce,s as q,t as le}from"./DatePicker-Ds9QkwJM.js";import{n as ue,t as de}from"./RadioGroup-C6GxtrOP.js";import{a as J}from"./index-De6-eCYx.js";import{t as fe}from"./_plugin-vue_export-helper-BDNMzG2s.js";var Y=`0!important`,X=`-1px!important`;function Z(e){return r(`${e}-type`,[c(`& +`,[t(`button`,{},[r(`${e}-type`,[d(`border`,{borderLeftWidth:Y}),d(`state-border`,{left:X})])])])])}function Q(e){return r(`${e}-type`,[c(`& +`,[t(`button`,[r(`${e}-type`,[d(`border`,{borderTopWidth:Y}),d(`state-border`,{top:X})])])])])}var pe=t(`button-group`,`
 flex-wrap: nowrap;
 display: inline-flex;
 position: relative;
`,[u(`vertical`,{flexDirection:`row`},[u(`rtl`,[t(`button`,[c(`&:first-child:not(:last-child)`,`
 margin-right: ${Y};
 border-top-right-radius: ${Y};
 border-bottom-right-radius: ${Y};
 `),c(`&:last-child:not(:first-child)`,`
 margin-left: ${Y};
 border-top-left-radius: ${Y};
 border-bottom-left-radius: ${Y};
 `),c(`&:not(:first-child):not(:last-child)`,`
 margin-left: ${Y};
 margin-right: ${Y};
 border-radius: ${Y};
 `),Z(`default`),r(`ghost`,[Z(`primary`),Z(`info`),Z(`success`),Z(`warning`),Z(`error`)])])])]),r(`vertical`,{flexDirection:`column`},[t(`button`,[c(`&:first-child:not(:last-child)`,`
 margin-bottom: ${Y};
 margin-left: ${Y};
 margin-right: ${Y};
 border-bottom-left-radius: ${Y};
 border-bottom-right-radius: ${Y};
 `),c(`&:last-child:not(:first-child)`,`
 margin-top: ${Y};
 margin-left: ${Y};
 margin-right: ${Y};
 border-top-left-radius: ${Y};
 border-top-right-radius: ${Y};
 `),c(`&:not(:first-child):not(:last-child)`,`
 margin: ${Y};
 border-radius: ${Y};
 `),Q(`default`),r(`ghost`,[Q(`primary`),Q(`info`),Q(`success`),Q(`warning`),Q(`error`)])])])]),me=b({name:`ButtonGroup`,props:{size:String,vertical:Boolean},setup(e){let{mergedClsPrefixRef:t,mergedRtlRef:n}=f(e);return I(`-button-group`,pe,t),y(ie,e),{rtlEnabled:h(`ButtonGroup`,n,t),mergedClsPrefix:t}},render(){let{mergedClsPrefix:e}=this;return S(),j(`div`,{class:o([`${e}-button-group`,this.rtlEnabled&&`${e}-button-group--rtl`,this.vertical&&`${e}-button-group--vertical`]),role:`group`},[p(()=>this.$slots.default?.())],2)}}),he=c([t(`calendar`,`
 line-height: var(--n-line-height);
 font-size: var(--n-font-size);
 color: var(--n-text-color);
 height: 720px;
 display: flex;
 flex-direction: column;
 `,[t(`calendar-prev-btn`,`
 cursor: pointer;
 `),t(`calendar-next-btn`,`
 cursor: pointer;
 `),t(`calendar-header`,`
 display: flex;
 align-items: center;
 line-height: 1;
 font-size: var(--n-title-font-size);
 padding: 0 0 18px 0;
 justify-content: space-between;
 `,[d(`title`,`
 color: var(--n-title-text-color);
 font-weight: var(--n-title-font-weight);
 transition: color .3s var(--n-bezier);
 `),d(`extra`,`
 display: flex;
 align-items: center;
 `)]),t(`calendar-dates`,`
 display: grid;
 grid-template-columns: repeat(7, minmax(0, 1fr));
 grid-auto-rows: 1fr;
 border-radius: var(--n-border-radius);
 flex: 1;
 border-top: 1px solid;
 border-left: 1px solid;
 border-color: var(--n-border-color);
 transition: border-color .3s var(--n-bezier);
 `),t(`calendar-cell`,`
 box-sizing: border-box;
 padding: 10px;
 border-right: 1px solid;
 border-bottom: 1px solid;
 border-color: var(--n-border-color);
 cursor: pointer;
 position: relative;
 transition:
 color .3s var(--n-bezier),
 border-color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 `,[c(`&:nth-child(7)`,`
 border-top-right-radius: var(--n-border-radius);
 `),c(`&:nth-last-child(7)`,`
 border-bottom-left-radius: var(--n-border-radius);
 `),c(`&:last-child`,`
 border-bottom-right-radius: var(--n-border-radius);
 `),c(`&:hover`,`
 background-color: var(--n-cell-color-hover);
 `),d(`bar`,`
 position: absolute;
 left: 0;
 right: 0;
 bottom: -1px;
 height: 3px;
 background-color: #0000;
 transition: background-color .3s var(--n-bezier);
 `),r(`selected`,[d(`bar`,`
 background-color: var(--n-bar-color);
 `)]),t(`calendar-date`,`
 transition:
 color .3s var(--n-bezier),
 border-color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 color: var(--n-text-color);
 `,[d(`date`,`
 color: var(--n-text-color);
 `)]),r(`disabled, other-month`,`
 color: var(--n-day-text-color);
 `,[t(`calendar-date`,[d(`date`,`
 color: var(--n-day-text-color);
 `)])]),r(`disabled`,`
 cursor: not-allowed;
 `),r(`current`,[t(`calendar-date`,[d(`date`,`
 color: var(--n-date-text-color-current);
 background-color: var(--n-date-color-current);
 `)])]),t(`calendar-date`,`
 position: relative;
 line-height: 1;
 display: flex;
 align-items: center;
 height: 1em;
 justify-content: space-between;
 padding-bottom: .75em;
 `,[d(`date`,`
 border-radius: 50%;
 display: flex;
 align-items: center;
 justify-content: center;
 margin-left: -0.4em;
 width: 1.8em;
 height: 1.8em;
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 `),d(`day`,`
 color: var(--n-day-text-color);
 transition: color .3s var(--n-bezier);
 `)])])]),i(t(`calendar`,[t(`calendar-dates`,`
 border-color: var(--n-border-color-modal);
 `),t(`calendar-cell`,`
 border-color: var(--n-border-color-modal);
 `,[c(`&:hover`,`
 background-color: var(--n-cell-color-hover-modal);
 `)])])),a(t(`calendar`,[t(`calendar-dates`,`
 border-color: var(--n-border-color-popover);
 `),t(`calendar-cell`,`
 border-color: var(--n-border-color-popover);
 `,[c(`&:hover`,`
 background-color: var(--n-cell-color-hover-popover);
 `)])]))]),ge=[`title`],_e=[`onClick`],ve=[`title`],ye={...n.props,isDateDisabled:Function,value:Number,defaultValue:{type:Number,default:null},onPanelChange:Function,"onUpdate:value":[Function,Array],onUpdateValue:[Function,Array]},be=b({name:`Calendar`,props:ye,slots:Object,setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:r}=f(e),i=n(`Calendar`,`-calendar`,he,R,e,t),{localeRef:a,dateLocaleRef:o}=L(`DatePicker`),s=Date.now(),c=T(q(e.defaultValue??s).valueOf()),u=T(e.defaultValue||null),d=re(x(e,`value`),u);function p(t,n){let{onUpdateValue:r,"onUpdate:value":i}=e;r&&m(r,t,n),i&&m(i,t,n),u.value=t}function h(){let t=G(c.value,-1).valueOf();c.value=t,e.onPanelChange?.({year:W(t),month:H(t)+1})}function g(){let t=G(c.value,1).valueOf();c.value=t,e.onPanelChange?.({year:W(t),month:H(t)+1})}function _(){let{value:t}=c,n=W(t),r=H(t),i=q(s).valueOf();c.value=i;let a=W(i),o=H(i);(n!==a||r!==o)&&e.onPanelChange?.({year:a,month:o+1})}let v=O(()=>{let{common:{cubicBezierEaseInOut:e},self:{borderColor:t,borderColorModal:n,borderColorPopover:r,borderRadius:a,titleFontSize:o,textColor:s,titleFontWeight:c,titleTextColor:l,dayTextColor:u,fontSize:d,lineHeight:f,dateColorCurrent:p,dateTextColorCurrent:m,cellColorHover:h,cellColor:g,cellColorModal:_,barColor:v,cellColorPopover:y,cellColorHoverModal:b,cellColorHoverPopover:x}}=i.value;return{"--n-bezier":e,"--n-border-color":t,"--n-border-color-modal":n,"--n-border-color-popover":r,"--n-border-radius":a,"--n-text-color":s,"--n-title-font-weight":c,"--n-title-font-size":o,"--n-title-text-color":l,"--n-day-text-color":u,"--n-font-size":d,"--n-line-height":f,"--n-date-color-current":p,"--n-date-text-color-current":m,"--n-cell-color":g,"--n-cell-color-modal":_,"--n-cell-color-popover":y,"--n-cell-color-hover":h,"--n-cell-color-hover-modal":b,"--n-cell-color-hover-popover":x,"--n-bar-color":v}}),y=r?l(`calendar`,void 0,v,e):void 0;return{mergedClsPrefix:t,locale:a,dateLocale:o,now:s,mergedValue:d,monthTs:c,dateItems:O(()=>ce(c.value,d.value,s,a.value.firstDayOfWeek,!0)),doUpdateValue:p,handleTodayClick:_,handlePrevClick:h,handleNextClick:g,mergedTheme:i,cssVars:r?void 0:v,themeClass:y?.themeClass,onRender:y?.onRender}},render(){let{isDateDisabled:e,mergedClsPrefix:t,monthTs:n,cssVars:r,mergedValue:i,mergedTheme:a,$slots:c,locale:{monthBeforeYear:l,today:u},dateLocale:{locale:d},handleTodayClick:f,handlePrevClick:m,handleNextClick:h,onRender:_}=this;_?.();let v=i&&U(i).valueOf(),y=W(n),b=H(n)+1;return S(),j(`div`,{class:o([`${t}-calendar`,this.themeClass]),style:w(r)},[A(`div`,{class:o(`${t}-calendar-header`)},[A(`div`,{class:o(`${t}-calendar-header__title`)},[p(()=>s(c.header,{year:y,month:b},()=>{let e=K(n,`MMMM`,{locale:d});return[l?`${e} ${y}`:`${y} ${e}`]}))],2),A(`div`,{class:o(`${t}-calendar-header__extra`)},[D(me,null,{default:()=>(S(),j(M,null,[(S(),N(z,{size:`small`,onClick:m,theme:a.peers.Button,themeOverrides:a.peerOverrides.Button},{icon:()=>(S(),N(g,{clsPrefix:t,class:o(`${t}-calendar-prev-btn`)},{default:()=>(S(),N(V))},1032,[`clsPrefix`,`class`]))},1032,[`onClick`,`theme`,`themeOverrides`])),(S(),N(z,{size:`small`,onClick:f,theme:a.peers.Button,themeOverrides:a.peerOverrides.Button},{default:()=>u},1032,[`onClick`,`theme`,`themeOverrides`])),(S(),N(z,{size:`small`,onClick:h,theme:a.peers.Button,themeOverrides:a.peerOverrides.Button},{icon:()=>(S(),N(g,{clsPrefix:t,class:o(`${t}-calendar-next-btn`)},{default:()=>(S(),N(se))},1032,[`clsPrefix`,`class`]))},1032,[`onClick`,`theme`,`themeOverrides`]))],64))},1024)],2)],2),A(`div`,{class:o(`${t}-calendar-dates`)},[p(()=>this.dateItems.map(({dateObject:n,ts:r,inCurrentMonth:i,isCurrentDate:a},s)=>{let{year:l,month:u,date:f}=n,m=K(r,`yyyy-MM-dd`),h=!i,g=e?.(r)===!0,_=v===U(r).valueOf();return S(),j(`div`,{key:`${b}-${s}`,class:o([`${t}-calendar-cell`,g&&`${t}-calendar-cell--disabled`,h&&`${t}-calendar-cell--other-month`,g&&`${t}-calendar-cell--not-allowed`,a&&`${t}-calendar-cell--current`,_&&`${t}-calendar-cell--selected`]),onClick:()=>{if(g)return;let e=q(r).valueOf();this.monthTs=e,h&&this.onPanelChange?.({year:W(e),month:H(e)+1}),this.doUpdateValue(r,{year:l,month:u+1,date:f})}},[A(`div`,{class:o(`${t}-calendar-date`)},[A(`div`,{class:o(`${t}-calendar-date__date`),title:m},[p(()=>f)],10,ve),p(()=>s<7&&(S(),j(`div`,{class:o(`${t}-calendar-date__day`),title:m},[p(()=>K(r,`EEE`,{locale:d}))],10,ge)))],2),p(()=>c.default?.({year:l,month:u+1,date:f})),A(`div`,{class:o(`${t}-calendar-cell__bar`)},null,2)],10,_e)}))],2)],6)}}),$=e(J(),1),xe={class:`page-content mb-5`},Se={class:`calendar-week-header`,"aria-hidden":`true`},Ce=[`onClick`],we={class:`flex flex-col gap-1 w-full min-h-0 flex-1 overflow-y-auto`},Te=[`onClick`],Ee={class:`dialog-footer`},De=fe(b({name:`TemplateCalendar`,__name:`index`,setup(e){let t=[{label:`基本`,value:`primary`},{label:`成功`,value:`success`},{label:`警告`,value:`warning`},{label:`危险`,value:`danger`}],n=[`一`,`二`,`三`,`四`,`五`,`六`,`日`],r=T((0,$.default)(`2025-02-07`)),i=T(!1),a=T(`添加事件`),o=T(-1),s=O({get:()=>r.value.valueOf(),set:e=>{r.value=(0,$.default)(e)}}),c=(e,t,n)=>`${e}-${String(t).padStart(2,`0`)}-${String(n).padStart(2,`0`)}`,l=T([{date:`2025-02-01`,content:`产品需求评审`,type:`primary`},{date:`2025-02-03`,endDate:`2025-02-05`,content:`项目周报会议（跨日期）`,type:`primary`},{date:`2025-02-10`,content:`瑜伽课程`,type:`success`},{date:`2025-02-15`,content:`团队建设活动`,type:`primary`},{date:`2025-02-20`,content:`健身训练`,type:`success`},{date:`2025-02-20`,content:`代码评审`,type:`danger`},{date:`2025-02-20`,content:`团队午餐`,type:`primary`},{date:`2025-02-20`,content:`项目进度汇报`,type:`warning`},{date:`2025-02-28`,content:`月度总结会`,type:`warning`}]),u=T({date:``,endDate:``,content:``,type:`primary`}),d=O(()=>o.value>=0),f=e=>u.value.date?e<(0,$.default)(u.value.date).startOf(`day`).valueOf():!1,p=(e=`primary`)=>({primary:{bgClass:`bg-theme/12`,textClass:`text-theme`},success:{bgClass:`bg-success/12`,textClass:`text-success`},warning:{bgClass:`bg-warning/12`,textClass:`text-warning`},danger:{bgClass:`bg-danger/12`,textClass:`text-danger`}})[e],m=e=>l.value.filter(t=>{let n=new Date(t.date),r=new Date(e),i=t.endDate?new Date(t.endDate):new Date(t.date);return r>=n&&r<=i}).map(e=>{let{bgClass:t,textClass:n}=p(e.type);return{...e,bgClass:t,textClass:n}}),h=()=>{u.value={date:``,endDate:``,content:``,type:`primary`},o.value=-1},g=e=>{a.value=`添加事件`,u.value={date:e,content:``,type:`primary`},o.value=-1,i.value=!0},y=e=>{a.value=`编辑事件`,u.value={...e},o.value=l.value.findIndex(t=>t.date===e.date&&t.content===e.content),i.value=!0},b=()=>{u.value.content&&u.value.date&&(d.value?l.value[o.value]={...u.value}:l.value.push({...u.value}),i.value=!1,h())},x=()=>{d.value&&(l.value.splice(o.value,1),i.value=!1,h())};return(e,r)=>{let o=be,l=oe,p=ae,w=ue,T=de,O=le,I=B,L=z,R=ne;return S(),j(`div`,xe,[A(`div`,Se,[(S(),j(M,null,C(n,e=>A(`span`,{key:e},v(e),1)),64))]),D(o,{value:_(s),"onUpdate:value":r[0]||(r[0]=e=>k(s)?s.value=e:null),"first-day-of-week":1},{default:E(({year:e,month:t,date:n})=>[A(`div`,{class:`calendar-cell-overlay`,onClick:r=>g(c(e,t,n))},[A(`div`,we,[(S(!0),j(M,null,C(m(c(e,t,n)),e=>(S(),j(`div`,{key:`${e.date}-${e.content}`,onClick:F(t=>y(e),[`stop`])},[A(`div`,{class:ee([`min-w-25 px-3 py-1.5 overflow-hidden text-xs/6 font-medium text-ellipsis whitespace-nowrap rounded hover:opacity-80`,[e.bgClass,e.textClass]])},v(e.content),3)],8,Te))),128))])],8,Ce)]),_:1},8,[`value`]),D(R,{show:_(i),"onUpdate:show":r[5]||(r[5]=e=>k(i)?i.value=e:null),preset:`card`,title:_(a),style:{width:`600px`},onAfterLeave:h},{footer:E(()=>[A(`span`,Ee,[_(d)?(S(),N(L,{key:0,type:`error`,onClick:x},{default:E(()=>[...r[6]||(r[6]=[P(` 删除 `,-1)])]),_:1})):te(``,!0),D(L,{type:`primary`,onClick:b},{default:E(()=>[P(v(_(d)?`更新`:`添加`),1)]),_:1})])]),default:E(()=>[D(I,{model:_(u),"label-width":`80px`},{default:E(()=>[D(p,{label:`活动标题`,required:``},{default:E(()=>[D(l,{value:_(u).content,"onUpdate:value":r[1]||(r[1]=e=>_(u).content=e),placeholder:`请输入活动标题`},null,8,[`value`])]),_:1}),D(p,{label:`事件颜色`},{default:E(()=>[D(T,{value:_(u).type,"onUpdate:value":r[2]||(r[2]=e=>_(u).type=e)},{default:E(()=>[(S(),j(M,null,C(t,e=>D(w,{key:e.value,value:e.value},{default:E(()=>[P(v(e.label),1)]),_:2},1032,[`value`])),64))]),_:1},8,[`value`])]),_:1}),D(p,{label:`开始日期`,required:``},{default:E(()=>[D(O,{style:{width:`100%`},"formatted-value":_(u).date,"onUpdate:formattedValue":r[3]||(r[3]=e=>_(u).date=e),placeholder:`选择日期`,"value-format":`yyyy-MM-dd`},null,8,[`formatted-value`])]),_:1}),D(p,{label:`结束日期`},{default:E(()=>[D(O,{style:{width:`100%`},"formatted-value":_(u).endDate,"onUpdate:formattedValue":r[4]||(r[4]=e=>_(u).endDate=e),placeholder:`选择结束日期`,"value-format":`yyyy-MM-dd`,"is-date-disabled":f},null,8,[`formatted-value`])]),_:1})]),_:1},8,[`model`])]),_:1},8,[`show`,`title`])])}}}),[[`__scopeId`,`data-v-f2476bf6`]]);export{De as default};