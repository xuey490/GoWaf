import{A as e,D as t,O as n,_ as r,at as i,c as a,ct as o,g as s,h as c,i as l,it as u,k as d,l as f,st as p,v as m,w as h,y as g}from"./resolve-slot-D0ESJhJD.js";import{n as _,t as v}from"./use-rtl-B41EZXdW.js";import{t as y}from"./download-DErx1K2W.js";import{st as b}from"./user-B1sGzif1.js";import{n as x,t as S}from"./replaceable-CG1Qs4lR.js";import{i as C,n as w,t as T}from"./Image-DQY_inse.js";import{H as ee,M as E,O as D,S as O,St as k,V as A,Wt as j,_t as M,d as N,f as P,h as F,i as I,it as L,j as R,p as z,s as te}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{t as B}from"./misc-DDs3MKLt.js";import{p as V}from"./vue-i18n-D4LRfH1J.js";import{i as ne,n as re,o as ie,t as ae}from"./icon-switch.cssr-o4i5UUsg.js";import{t as oe}from"./use-merged-state-KQOIh2IK.js";import{d as H,t as U}from"./Button-DUNxlS0h.js";import{n as se}from"./Input-ByrqO_2o.js";import{t as ce}from"./is-browser-e-NGrvPW.js";import{t as le}from"./Add-DQqGLM7W.js";import{t as ue}from"./Progress-CBh21Xn0.js";var W=t(`n-upload`),G=O({name:`UploadDragger`,__UPLOAD_DRAGGER__:!0,setup(e,{slots:t}){let n=D(W,null);return n||d(`upload-dragger`,"`n-upload-dragger` must be placed inside `n-upload`."),()=>{let{mergedClsPrefixRef:{value:e},mergedDisabledRef:{value:r},maxReachedRef:{value:i}}=n;return A(),F(`div`,{class:s([`${e}-upload-dragger`,(r||i)&&`${e}-upload-dragger--disabled`])},[g(()=>t.default?.())],2)}}});function K(e){return e.includes(`image/`)}function q(e=``){let t=e.split(`/`),n=t[t.length-1].split(/#|\?/)[0];return(/\.[^./\\]*$/.exec(n)||[``])[0]}var J=/(webp|svg|png|gif|jpg|jpeg|jfif|bmp|dpg|ico)$/i,Y=e=>{if(e.type)return K(e.type);let t=q(e.name||``);if(J.test(t))return!0;let n=e.thumbnailUrl||e.url||``,r=q(n);return!!(/^data:image\//.test(n)||J.test(r))};async function de(e){return await new Promise(t=>{!e.type||!K(e.type)?t(``):t(window.URL.createObjectURL(e))})}var fe=ce&&window.FileReader&&window.File;function pe(e){return e.isDirectory}function me(e){return e.isFile}async function he(e,t){let r=[];async function i(e){for(let a of e)if(a){if(t&&pe(a)){let e=a.createReader(),t=[],r;try{do r=await new Promise((t,n)=>{e.readEntries(t,n)}),t=t.concat(r);while(r.length>0)}catch(e){n(`upload`,`error happens when handling directory upload`,e)}await i(t)}else if(me(a))try{let e=await new Promise((e,t)=>{a.file(e,t)});r.push({file:e,entry:a,source:`dnd`})}catch(e){n(`upload`,`error happens when handling file upload`,e)}}}return await i(e),r}function X(e){let{id:t,name:n,percentage:r,status:i,url:a,file:o,thumbnailUrl:s,type:c,fullPath:l,batchId:u}=e;return{id:t,name:n,percentage:r??null,status:i,url:a??null,file:o??null,thumbnailUrl:s??null,type:c??null,fullPath:l??null,batchId:u??null}}function ge(e,t,n){return e=e.toLowerCase(),t=t.toLocaleLowerCase(),n=n.toLocaleLowerCase(),n.split(`,`).map(e=>e.trim()).filter(Boolean).some(n=>{if(n.startsWith(`.`)){if(e.endsWith(n))return!0}else if(n.includes(`/`)){let[e,r]=t.split(`/`),[i,a]=n.split(`/`);if((i===`*`||e&&i&&i===e)&&(a===`*`||r&&a&&a===r))return!0}else return!0;return!1})}var _e=[`onClick`,`onDrop`,`onDragover`,`onDragenter`,`onDragleave`],Z=O({name:`UploadTrigger`,props:{abstract:Boolean},slots:Object,setup(e,{slots:t}){let n=D(W,null);n||d(`upload-trigger`,"`n-upload-trigger` must be placed inside `n-upload`.");let{mergedClsPrefixRef:r,mergedDisabledRef:i,maxReachedRef:a,listTypeRef:o,dragOverRef:c,openOpenFileDialog:u,draggerInsideRef:f,handleFileAddition:p,mergedDirectoryDndRef:m,triggerClassRef:h,triggerStyleRef:_}=n,v=N(()=>o.value===`image-card`);function y(){i.value||a.value||u()}function b(e){e.preventDefault(),c.value=!0}function S(e){e.preventDefault(),c.value=!0}function C(e){e.preventDefault(),c.value=!1}function w(e){if(e.preventDefault(),!f.value||i.value||a.value){c.value=!1;return}let t=e.dataTransfer?.items;t?.length?he(Array.from(t).map(e=>e.webkitGetAsEntry()),m.value).then(e=>{p(e)}).finally(()=>{c.value=!1}):c.value=!1}return()=>{let{value:n}=r,o={handleClick:y,handleDrop:w,handleDragOver:b,handleDragEnter:S,handleDragLeave:C};return e.abstract?t.default?.(o):(A(),F(`div`,{key:1,class:s([`${n}-upload-trigger`,(i.value||a.value)&&`${n}-upload-trigger--disabled`,v.value&&`${n}-upload-trigger--image-card`,h.value]),style:j(_.value),onClick:y,onDrop:w,onDragover:b,onDragenter:S,onDragleave:C},[v.value?(A(),z(G,{key:0},{default:()=>l(t.default,o,()=>[(A(),z(x,{clsPrefix:n},{default:()=>(A(),z(le))},1032,[`clsPrefix`]))])},1024)):(A(),F(I,{key:1},[g(()=>t.default?.(o))],64))],46,_e))}}}),ve=S(`attach`,()=>(()=>{let e=c(`5efcd6da8c4f9f81`);return e[0]||(e[0]=P(`svg`,{viewBox:`0 0 16 16`,version:`1.1`,xmlns:`http://www.w3.org/2000/svg`},[P(`g`,{stroke:`none`,"stroke-width":`1`,fill:`none`,"fill-rule":`evenodd`},[P(`g`,{fill:`currentColor`,"fill-rule":`nonzero`},[P(`path`,{d:`M3.25735931,8.70710678 L7.85355339,4.1109127 C8.82986412,3.13460197 10.4127766,3.13460197 11.3890873,4.1109127 C12.365398,5.08722343 12.365398,6.67013588 11.3890873,7.64644661 L6.08578644,12.9497475 C5.69526215,13.3402718 5.06209717,13.3402718 4.67157288,12.9497475 C4.28104858,12.5592232 4.28104858,11.9260582 4.67157288,11.5355339 L9.97487373,6.23223305 C10.1701359,6.0369709 10.1701359,5.72038841 9.97487373,5.52512627 C9.77961159,5.32986412 9.4630291,5.32986412 9.26776695,5.52512627 L3.96446609,10.8284271 C3.18341751,11.6094757 3.18341751,12.8758057 3.96446609,13.6568542 C4.74551468,14.4379028 6.01184464,14.4379028 6.79289322,13.6568542 L12.0961941,8.35355339 C13.4630291,6.98671837 13.4630291,4.77064094 12.0961941,3.40380592 C10.7293591,2.0369709 8.51328163,2.0369709 7.14644661,3.40380592 L2.55025253,8 C2.35499039,8.19526215 2.35499039,8.51184464 2.55025253,8.70710678 C2.74551468,8.90236893 3.06209717,8.90236893 3.25735931,8.70710678 Z`})])])],-1))})()),ye=S(`cancel`,()=>(()=>{let e=c(`32793e1b4a60bf1b`);return e[0]||(e[0]=P(`svg`,{viewBox:`0 0 16 16`,version:`1.1`,xmlns:`http://www.w3.org/2000/svg`},[P(`g`,{stroke:`none`,"stroke-width":`1`,fill:`none`,"fill-rule":`evenodd`},[P(`g`,{fill:`currentColor`,"fill-rule":`nonzero`},[P(`path`,{d:`M2.58859116,2.7156945 L2.64644661,2.64644661 C2.82001296,2.47288026 3.08943736,2.45359511 3.2843055,2.58859116 L3.35355339,2.64644661 L8,7.293 L12.6464466,2.64644661 C12.8417088,2.45118446 13.1582912,2.45118446 13.3535534,2.64644661 C13.5488155,2.84170876 13.5488155,3.15829124 13.3535534,3.35355339 L8.707,8 L13.3535534,12.6464466 C13.5271197,12.820013 13.5464049,13.0894374 13.4114088,13.2843055 L13.3535534,13.3535534 C13.179987,13.5271197 12.9105626,13.5464049 12.7156945,13.4114088 L12.6464466,13.3535534 L8,8.707 L3.35355339,13.3535534 C3.15829124,13.5488155 2.84170876,13.5488155 2.64644661,13.3535534 C2.45118446,13.1582912 2.45118446,12.8417088 2.64644661,12.6464466 L7.293,8 L2.64644661,3.35355339 C2.47288026,3.17998704 2.45359511,2.91056264 2.58859116,2.7156945 L2.64644661,2.64644661 L2.58859116,2.7156945 Z`})])])],-1))})()),be=S(`retry`,()=>(()=>{let e=c(`32d3a81cd807b3fd`);return e[0]||(e[0]=P(`svg`,{xmlns:`http://www.w3.org/2000/svg`,viewBox:`0 0 512 512`},[P(`path`,{d:`M320,146s24.36-12-64-12A160,160,0,1,0,416,294`,style:`fill: none; stroke: currentcolor; stroke-linecap: round; stroke-miterlimit: 10; stroke-width: 32px;`}),P(`polyline`,{points:`256 58 336 138 256 218`,style:`fill: none; stroke: currentcolor; stroke-linecap: round; stroke-linejoin: round; stroke-width: 32px;`})],-1))})()),xe=S(`trash`,()=>(()=>{let e=c(`4bd1b3f5f0c823a2`);return e[0]||(e[0]=P(`svg`,{xmlns:`http://www.w3.org/2000/svg`,viewBox:`0 0 512 512`},[P(`path`,{d:`M432,144,403.33,419.74A32,32,0,0,1,371.55,448H140.46a32,32,0,0,1-31.78-28.26L80,144`,style:`fill: none; stroke: currentcolor; stroke-linecap: round; stroke-linejoin: round; stroke-width: 32px;`}),P(`rect`,{x:`32`,y:`64`,width:`448`,height:`80`,rx:`16`,ry:`16`,style:`fill: none; stroke: currentcolor; stroke-linecap: round; stroke-linejoin: round; stroke-width: 32px;`}),P(`line`,{x1:`312`,y1:`240`,x2:`200`,y2:`352`,style:`fill: none; stroke: currentcolor; stroke-linecap: round; stroke-linejoin: round; stroke-width: 32px;`}),P(`line`,{x1:`312`,y1:`352`,x2:`200`,y2:`240`,style:`fill: none; stroke: currentcolor; stroke-linecap: round; stroke-linejoin: round; stroke-width: 32px;`})],-1))})());function Se(){return(()=>{let e=c(`39989d0be5c9bff7`);return e[0]||(e[0]=P(`svg`,{xmlns:`http://www.w3.org/2000/svg`,viewBox:`0 0 28 28`},[P(`g`,{fill:`none`},[P(`path`,{d:`M21.75 3A3.25 3.25 0 0 1 25 6.25v15.5A3.25 3.25 0 0 1 21.75 25H6.25A3.25 3.25 0 0 1 3 21.75V6.25A3.25 3.25 0 0 1 6.25 3h15.5zm.583 20.4l-7.807-7.68a.75.75 0 0 0-.968-.07l-.084.07l-7.808 7.68c.183.065.38.1.584.1h15.5c.204 0 .4-.035.583-.1l-7.807-7.68l7.807 7.68zM21.75 4.5H6.25A1.75 1.75 0 0 0 4.5 6.25v15.5c0 .208.036.408.103.593l7.82-7.692a2.25 2.25 0 0 1 3.026-.117l.129.117l7.82 7.692c.066-.185.102-.385.102-.593V6.25a1.75 1.75 0 0 0-1.75-1.75zm-3.25 3a2.5 2.5 0 1 1 0 5a2.5 2.5 0 0 1 0-5zm0 1.5a1 1 0 1 0 0 2a1 1 0 0 0 0-2z`,fill:`currentColor`})])],-1))})()}function Ce(){return(()=>{let e=c(`c8e04259929228b8`);return e[0]||(e[0]=P(`svg`,{xmlns:`http://www.w3.org/2000/svg`,viewBox:`0 0 28 28`},[P(`g`,{fill:`none`},[P(`path`,{d:`M6.4 2A2.4 2.4 0 0 0 4 4.4v19.2A2.4 2.4 0 0 0 6.4 26h15.2a2.4 2.4 0 0 0 2.4-2.4V11.578c0-.729-.29-1.428-.805-1.944l-6.931-6.931A2.4 2.4 0 0 0 14.567 2H6.4zm-.9 2.4a.9.9 0 0 1 .9-.9H14V10a2 2 0 0 0 2 2h6.5v11.6a.9.9 0 0 1-.9.9H6.4a.9.9 0 0 1-.9-.9V4.4zm16.44 6.1H16a.5.5 0 0 1-.5-.5V4.06l6.44 6.44z`,fill:`currentColor`})])],-1))})()}var we=O({name:`UploadProgress`,props:{show:Boolean,percentage:{type:Number,required:!0},status:{type:String,required:!0}},setup(){return{mergedTheme:D(W).mergedThemeRef}},render(){return A(),z(H,null,{_:1,default:r(()=>this.show?(A(),z(ue,{key:1,type:`line`,showIndicator:!1,percentage:this.percentage,status:this.status,height:2,theme:this.mergedTheme.peers.Progress,themeOverrides:this.mergedTheme.peerOverrides.Progress},null,8,[`percentage`,`status`,`theme`,`themeOverrides`])):null)})}}),Te=[`href`,`onClick`],Ee=[`href`,`onClick`],De=[`onClick`],Q={paddingMedium:`0 3px`,heightMedium:`24px`,iconSizeMedium:`18px`},Oe=O({name:`UploadFile`,props:{clsPrefix:{type:String,required:!0},file:{type:Object,required:!0},listType:{type:String,required:!0},index:{type:Number,required:!0}},setup(t){let n=D(W),r=M(null),i=M(``),a=N(()=>{let{file:e}=t;return e.status===`finished`?`success`:e.status===`error`?`error`:`info`}),o=N(()=>{let{file:e}=t;if(e.status===`error`)return`error`}),s=N(()=>{let{file:e}=t;return e.status===`uploading`}),c=N(()=>{if(!n.showCancelButtonRef.value)return!1;let{file:e}=t;return[`uploading`,`pending`,`error`].includes(e.status)}),l=N(()=>{if(!n.showRemoveButtonRef.value)return!1;let{file:e}=t;return[`finished`].includes(e.status)}),u=N(()=>{if(!n.showDownloadButtonRef.value)return!1;let{file:e}=t;return[`finished`].includes(e.status)}),d=N(()=>{if(!n.showRetryButtonRef.value)return!1;let{file:e}=t;return[`error`].includes(e.status)}),f=ie(()=>i.value||t.file.thumbnailUrl||t.file.url),p=N(()=>{if(!n.showPreviewButtonRef.value)return!1;let{file:{status:e},listType:r}=t;return[`finished`].includes(e)&&f.value&&r===`image-card`});async function m(){let e=n.onRetryRef.value;e&&await e({file:t.file})===!1||n.submit({fileId:t.file.id})}function h(n){n.preventDefault();let{file:r}=t;[`finished`,`pending`,`error`].includes(r.status)?_(r):[`uploading`].includes(r.status)?b(r):e(`upload`,`The button clicked type is unknown.`)}function g(e){e.preventDefault(),v(t.file)}function _(e){let{xhrMap:r,doChange:i,onRemoveRef:{value:a},mergedFileListRef:{value:o}}=n;Promise.resolve(!a||a({file:Object.assign({},e),fileList:o,index:t.index})).then(t=>{if(t===!1)return;let n=Object.assign({},e,{status:`removed`});r.delete(e.id),i(n,void 0,{remove:!0})})}function v(e){let{onDownloadRef:{value:t},customDownloadRef:{value:r}}=n;Promise.resolve(!t||t(Object.assign({},e))).then(t=>{t!==!1&&(r?r(Object.assign({},e)):y(e.url,e.name))})}function b(e){let{xhrMap:t}=n;t.get(e.id)?.abort(),_(Object.assign({},e))}function x(e){let{onPreviewRef:{value:i}}=n;if(i)i(t.file,{event:e});else if(t.listType===`image-card`){let{value:e}=r;if(!e)return;e.showPreview()}}let S=async()=>{let{listType:e}=t;(e===`image`||e===`image-card`)&&n.shouldUseThumbnailUrlRef.value(t.file)&&(i.value=await n.getFileThumbnailUrlResolver(t.file))};return L(()=>{S()}),{mergedTheme:n.mergedThemeRef,progressStatus:a,buttonType:o,showProgress:s,disabled:n.mergedDisabledRef,showCancelButton:c,showRemoveButton:l,showDownloadButton:u,showRetryButton:d,showPreviewButton:p,alwaysShowActions:n.alwaysShowActionsRef,mergedThumbnailUrl:f,shouldUseThumbnailUrl:n.shouldUseThumbnailUrlRef,renderIcon:n.renderIconRef,imageRef:r,handleRemoveOrCancelClick:h,handleDownloadClick:g,handleRetryClick:m,handlePreviewClick:x}},render(){let{clsPrefix:e,mergedTheme:t,listType:n,file:r,renderIcon:i}=this,a,o=n===`image`;a=o||n===`image-card`?!this.shouldUseThumbnailUrl(r)||!this.mergedThumbnailUrl?(A(),F(`span`,{key:1,class:s(`${e}-upload-file-info__thumbnail`)},[i?(A(),F(I,{key:0},[g(()=>i(r))],64)):(A(),F(I,{key:1},[Y(r)?(A(),z(x,{key:0,clsPrefix:e},{default:Se},1032,[`clsPrefix`])):(A(),z(x,{key:1,clsPrefix:e},{default:Ce},1032,[`clsPrefix`]))],64))],2)):(A(),F(`a`,{key:2,rel:`noopener noreferer`,target:`_blank`,href:r.url||void 0,class:s(`${e}-upload-file-info__thumbnail`),onClick:this.handlePreviewClick},[n===`image-card`?(A(),z(T,{key:0,src:this.mergedThumbnailUrl||void 0,previewSrc:r.url||void 0,alt:r.name,ref:`imageRef`},null,8,[`src`,`previewSrc`,`alt`])):(A(),F(`img`,{key:1,src:this.mergedThumbnailUrl||void 0,alt:r.name},null,8,[`src`,`alt`]))],10,Te)):(t=>(A(),F(`span`,{key:3,class:s(`${e}-upload-file-info__thumbnail`)},[i?(A(),F(I,{key:0},[g(()=>i(r))],64)):(A(),z(x,{key:1,clsPrefix:e},{default:()=>(A(),z(ve))},1032,[`clsPrefix`]))],2)))(a);let c=(A(),z(we,{show:this.showProgress,percentage:r.percentage||0,status:this.progressStatus},null,8,[`show`,`percentage`,`status`])),l=n===`text`||n===`image`;return A(),F(`div`,{class:s([`${e}-upload-file`,`${e}-upload-file--${this.progressStatus}-status`,r.url&&r.status!==`error`&&n!==`image-card`&&`${e}-upload-file--with-url`,`${e}-upload-file--${n}-type`,this.alwaysShowActions&&`${e}-upload-file--always-show-actions`])},[P(`div`,{class:s(`${e}-upload-file-info`)},[g(()=>a),P(`div`,{class:s(`${e}-upload-file-info__name`)},[g(()=>l&&(r.url&&r.status!==`error`?(A(),F(`a`,{key:4,rel:`noopener noreferer`,target:`_blank`,href:r.url||void 0,onClick:this.handlePreviewClick},[g(()=>r.name)],8,Ee)):(A(),F(`span`,{key:5,onClick:this.handlePreviewClick},[g(()=>r.name)],8,De)))),g(()=>o&&c)],2),P(`div`,{class:s([`${e}-upload-file-info__action`,`${e}-upload-file-info__action--${n}-type`,this.alwaysShowActions&&`${e}-upload-file-info__action--always-show`])},[this.showPreviewButton?(A(),z(U,{key:`preview`,quaternary:!0,type:this.buttonType,onClick:this.handlePreviewClick,theme:t.peers.Button,themeOverrides:t.peerOverrides.Button,builtinThemeOverrides:Q},{icon:()=>(A(),z(x,{clsPrefix:e},{default:()=>(A(),z(se))},1032,[`clsPrefix`]))},1032,[`type`,`onClick`,`theme`,`themeOverrides`,`builtinThemeOverrides`])):g(()=>null),g(()=>(this.showRemoveButton||this.showCancelButton)&&!this.disabled&&(A(),z(U,{key:`cancelOrTrash`,theme:t.peers.Button,themeOverrides:t.peerOverrides.Button,quaternary:!0,builtinThemeOverrides:Q,type:this.buttonType,onClick:this.handleRemoveOrCancelClick},{icon:()=>(A(),z(re,null,{default:()=>this.showRemoveButton?(A(),z(x,{clsPrefix:e,key:`trash`},{default:()=>(A(),z(xe))},1032,[`clsPrefix`])):(A(),z(x,{clsPrefix:e,key:`cancel`},{default:()=>(A(),z(ye))},1032,[`clsPrefix`]))},1024))},1032,[`theme`,`themeOverrides`,`builtinThemeOverrides`,`type`,`onClick`]))),g(()=>this.showRetryButton&&!this.disabled&&(A(),z(U,{key:`retry`,quaternary:!0,type:this.buttonType,onClick:this.handleRetryClick,theme:t.peers.Button,themeOverrides:t.peerOverrides.Button,builtinThemeOverrides:Q},{icon:()=>(A(),z(x,{clsPrefix:e},{default:()=>(A(),z(be))},1032,[`clsPrefix`]))},1032,[`type`,`onClick`,`theme`,`themeOverrides`,`builtinThemeOverrides`]))),this.showDownloadButton?(A(),z(U,{key:`download`,quaternary:!0,type:this.buttonType,onClick:this.handleDownloadClick,theme:t.peers.Button,themeOverrides:t.peerOverrides.Button,builtinThemeOverrides:Q},{icon:()=>(A(),z(x,{clsPrefix:e},{default:()=>(A(),z(C))},1032,[`clsPrefix`]))},1032,[`type`,`onClick`,`theme`,`themeOverrides`,`builtinThemeOverrides`])):g(()=>null)],2)],2),g(()=>!o&&c)],2)}}),ke=O({name:`UploadFileList`,setup(e,{slots:t}){let n=D(W,null);n||d(`upload-file-list`,"`n-upload-file-list` must be placed inside `n-upload`.");let{abstractRef:r,mergedClsPrefixRef:i,listTypeRef:a,mergedFileListRef:o,fileListClassRef:c,fileListStyleRef:l,cssVarsRef:u,themeClassRef:f,maxReachedRef:p,showTriggerRef:h,imageGroupPropsRef:_}=n,v=N(()=>a.value===`image-card`),y=()=>o.value.map((e,t)=>(A(),z(Oe,{clsPrefix:i.value,key:e.id,file:e,index:t,listType:a.value},null,8,[`clsPrefix`,`file`,`index`,`listType`]))),b=()=>v.value?(A(),z(w,R({key:1},_.value),{default:y},1040)):(A(),z(H,{key:2,group:!0},{default:y},1024));return()=>{let{value:e}=i,{value:n}=r;return A(),F(`div`,{class:s([`${e}-upload-file-list`,v.value&&`${e}-upload-file-list--grid`,n?f?.value:void 0,c.value]),style:j([n&&u?u.value:``,l.value])},[g(()=>b()),g(()=>h.value&&!p.value&&v.value&&(A(),z(Z,null,m(t),1024)))],6)}}}),Ae=u([i(`upload`,`width: 100%;`,[o(`dragger-inside`,[i(`upload-trigger`,`
 display: block;
 `)]),o(`drag-over`,[i(`upload-dragger`,`
 border: var(--n-dragger-border-hover);
 `)])]),i(`upload-dragger`,`
 cursor: pointer;
 box-sizing: border-box;
 width: 100%;
 text-align: center;
 border-radius: var(--n-border-radius);
 padding: 24px;
 opacity: 1;
 transition:
 opacity .3s var(--n-bezier),
 border-color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 background-color: var(--n-dragger-color);
 border: var(--n-dragger-border);
 `,[u(`&:hover`,`
 border: var(--n-dragger-border-hover);
 `),o(`disabled`,`
 cursor: not-allowed;
 `)]),i(`upload-trigger`,`
 display: inline-block;
 box-sizing: border-box;
 opacity: 1;
 transition: opacity .3s var(--n-bezier);
 `,[u(`+`,[i(`upload-file-list`,`margin-top: 8px;`)]),o(`disabled`,`
 opacity: var(--n-item-disabled-opacity);
 cursor: not-allowed;
 `),o(`image-card`,`
 width: 96px;
 height: 96px;
 `,[i(`base-icon`,`
 font-size: 24px;
 `),i(`upload-dragger`,`
 padding: 0;
 height: 100%;
 width: 100%;
 display: flex;
 align-items: center;
 justify-content: center;
 `)])]),i(`upload-file-list`,`
 line-height: var(--n-line-height);
 opacity: 1;
 transition: opacity .3s var(--n-bezier);
 `,[u(`a, img`,`outline: none;`),o(`disabled`,`
 opacity: var(--n-item-disabled-opacity);
 cursor: not-allowed;
 `,[i(`upload-file`,`cursor: not-allowed;`)]),o(`grid`,`
 display: grid;
 grid-template-columns: repeat(auto-fill, 96px);
 grid-gap: 8px;
 margin-top: 0;
 `),i(`upload-file`,`
 display: block;
 box-sizing: border-box;
 cursor: default;
 padding: 0px 12px 0 6px;
 transition: background-color .3s var(--n-bezier);
 border-radius: var(--n-border-radius);
 `,[V(),i(`progress`,[V({foldPadding:!0})]),u(`&:hover`,`
 background-color: var(--n-item-color-hover);
 `,[i(`upload-file-info`,[p(`action`,`
 opacity: 1;
 `)])]),o(`image-type`,`
 border-radius: var(--n-border-radius);
 text-decoration: underline;
 text-decoration-color: #0000;
 `,[i(`upload-file-info`,`
 padding-top: 0px;
 padding-bottom: 0px;
 width: 100%;
 height: 100%;
 display: flex;
 justify-content: space-between;
 align-items: center;
 padding: 6px 0;
 `,[i(`progress`,`
 padding: 2px 0;
 margin-bottom: 0;
 `),p(`name`,`
 padding: 0 8px;
 `),p(`thumbnail`,`
 width: 32px;
 height: 32px;
 font-size: 28px;
 display: flex;
 justify-content: center;
 align-items: center;
 `,[u(`img`,`
 width: 100%;
 `)])])]),o(`text-type`,[i(`progress`,`
 box-sizing: border-box;
 padding-bottom: 6px;
 margin-bottom: 6px;
 `)]),o(`image-card-type`,`
 position: relative;
 width: 96px;
 height: 96px;
 border: var(--n-item-border-image-card);
 border-radius: var(--n-border-radius);
 padding: 0;
 display: flex;
 align-items: center;
 justify-content: center;
 transition: border-color .3s var(--n-bezier), background-color .3s var(--n-bezier);
 border-radius: var(--n-border-radius);
 overflow: hidden;
 `,[i(`progress`,`
 position: absolute;
 left: 8px;
 bottom: 8px;
 right: 8px;
 width: unset;
 `),i(`upload-file-info`,`
 padding: 0;
 width: 100%;
 height: 100%;
 `,[p(`thumbnail`,`
 width: 100%;
 height: 100%;
 display: flex;
 flex-direction: column;
 align-items: center;
 justify-content: center;
 font-size: 36px;
 `,[u(`img`,`
 width: 100%;
 `)])]),u(`&::before`,`
 position: absolute;
 z-index: 1;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 border-radius: inherit;
 opacity: 0;
 transition: opacity .2s var(--n-bezier);
 content: "";
 `),u(`&:hover`,[u(`&::before`,`opacity: 1;`),i(`upload-file-info`,[p(`thumbnail`,`opacity: .12;`)])]),o(`always-show-actions`,[u(`&::before`,`opacity: 1;`),i(`upload-file-info`,[p(`thumbnail`,`opacity: .12;`)])])]),o(`error-status`,[u(`&:hover`,`
 background-color: var(--n-item-color-hover-error);
 `),i(`upload-file-info`,[p(`name`,`color: var(--n-item-text-color-error);`),p(`thumbnail`,`color: var(--n-item-text-color-error);`)]),o(`image-card-type`,`
 border: var(--n-item-border-image-card-error);
 `)]),o(`with-url`,`
 cursor: pointer;
 `,[i(`upload-file-info`,[p(`name`,`
 color: var(--n-item-text-color-success);
 text-decoration-color: var(--n-item-text-color-success);
 `,[u(`a`,`
 text-decoration: underline;
 `)])])]),i(`upload-file-info`,`
 position: relative;
 padding-top: 6px;
 padding-bottom: 6px;
 display: flex;
 flex-wrap: nowrap;
 `,[p(`thumbnail`,`
 font-size: 18px;
 opacity: 1;
 transition: opacity .2s var(--n-bezier);
 color: var(--n-item-icon-color);
 `,[i(`base-icon`,`
 margin-right: 2px;
 vertical-align: middle;
 transition: color .3s var(--n-bezier);
 `)]),p(`action`,`
 padding-top: inherit;
 padding-bottom: inherit;
 position: absolute;
 right: 0;
 top: 0;
 bottom: 0;
 width: 80px;
 display: flex;
 align-items: center;
 transition: opacity .2s var(--n-bezier);
 justify-content: flex-end;
 opacity: 0;
 `,[i(`button`,[u(`&:not(:last-child)`,{marginRight:`4px`}),i(`base-icon`,[u(`svg`,[ae()])])]),o(`image-type`,`
 position: relative;
 max-width: 80px;
 width: auto;
 `),o(`image-card-type`,`
 z-index: 2;
 position: absolute;
 width: 100%;
 height: 100%;
 left: 0;
 right: 0;
 bottom: 0;
 top: 0;
 display: flex;
 justify-content: center;
 align-items: center;
 `),o(`always-show`,`
 opacity: 1;
 `)]),p(`name`,`
 color: var(--n-item-text-color);
 flex: 1;
 display: flex;
 justify-content: center;
 text-overflow: ellipsis;
 overflow: hidden;
 flex-direction: column;
 text-decoration-color: #0000;
 font-size: var(--n-font-size);
 transition:
 color .3s var(--n-bezier),
 text-decoration-color .3s var(--n-bezier); 
 `,[u(`a`,`
 color: inherit;
 text-decoration: underline;
 `)])])])]),i(`upload-file-input`,`
 display: none;
 width: 0;
 height: 0;
 opacity: 0;
 `)]),je=[`accept`,`multiple`,`onChange`,`webkitdirectory`,`directory`];function Me(e,t,n){let{doChange:r,xhrMap:i}=e,a=0;function o(n){let o=Object.assign({},t,{status:`error`,percentage:a});i.delete(t.id),o=X(e.onError?.({file:o,event:n})||o),r(o,n)}function s(s){if(e.isErrorState){if(e.isErrorState(n)){o(s);return}}else if(n.status<200||n.status>=300){o(s);return}let c=Object.assign({},t,{status:`finished`,percentage:a});i.delete(t.id),c=X(e.onFinish?.({file:c,event:s})||c),r(c,s)}return{handleXHRLoad:s,handleXHRError:o,handleXHRAbort(e){let n=Object.assign({},t,{status:`removed`,file:null,percentage:a});i.delete(t.id),r(n,e)},handleXHRProgress(e){let n=Object.assign({},t,{status:`uploading`});if(e.lengthComputable){let t=Math.ceil(e.loaded/e.total*100);n.percentage=t,a=t}r(n,e)}}}function Ne(e){let{inst:t,file:n,data:r,headers:i,withCredentials:a,action:o,customRequest:s}=e,{doChange:c}=e.inst,l=0;s({file:n,data:r,headers:i,withCredentials:a,action:o,onProgress(e){let t=Object.assign({},n,{status:`uploading`}),r=e.percent;t.percentage=r,l=r,c(t)},onFinish(){let e=Object.assign({},n,{status:`finished`,percentage:l});e=X(t.onFinish?.({file:e})||e),c(e)},onError(){let e=Object.assign({},n,{status:`error`,percentage:l});e=X(t.onError?.({file:e})||e),c(e)}})}function Pe(e,t,n){let r=Me(e,t,n);n.onabort=r.handleXHRAbort,n.onerror=r.handleXHRError,n.onload=r.handleXHRLoad,n.upload&&(n.upload.onprogress=r.handleXHRProgress)}function $(e,t){return typeof e==`function`?e({file:t}):e||{}}function Fe(e,t,n){let r=$(t,n);r&&Object.keys(r).forEach(t=>{e.setRequestHeader(t,r[t])})}function Ie(e,t,n){let r=$(t,n);r&&Object.keys(r).forEach(t=>{e.append(t,r[t])})}function Le(e,t,n,{method:r,action:i,withCredentials:a,responseType:o,headers:s,data:c}){let l=new XMLHttpRequest;l.responseType=o,e.xhrMap.set(n.id,l),l.withCredentials=a;let u=new FormData;if(Ie(u,c,n),n.file!==null&&u.append(t,n.file),Pe(e,n,l),i!==void 0){l.open(r.toUpperCase(),i),Fe(l,s,n),l.send(u);let t=Object.assign({},n,{status:`uploading`});e.doChange(t)}}var Re={...a.props,name:{type:String,default:`file`},accept:String,action:String,customRequest:Function,directory:Boolean,directoryDnd:{type:Boolean,default:void 0},method:{type:String,default:`POST`},multiple:Boolean,showFileList:{type:Boolean,default:!0},data:[Object,Function],headers:[Object,Function],withCredentials:Boolean,responseType:{type:String,default:``},disabled:{type:Boolean,default:void 0},onChange:Function,onRemove:Function,onFinish:Function,onError:Function,onRetry:Function,onBeforeUpload:Function,isErrorState:Function,onDownload:Function,customDownload:Function,defaultUpload:{type:Boolean,default:!0},fileList:Array,"onUpdate:fileList":[Function,Array],onUpdateFileList:[Function,Array],fileListClass:String,fileListStyle:[String,Object],defaultFileList:{type:Array,default:()=>[]},showCancelButton:{type:Boolean,default:!0},showRemoveButton:{type:Boolean,default:!0},showDownloadButton:Boolean,showRetryButton:{type:Boolean,default:!0},showPreviewButton:{type:Boolean,default:!0},alwaysShowActions:Boolean,listType:{type:String,default:`text`},onPreview:Function,shouldUseThumbnailUrl:{type:Function,default:e=>fe?Y(e):!1},createThumbnailUrl:Function,abstract:Boolean,max:Number,showTrigger:{type:Boolean,default:!0},imageGroupProps:Object,inputProps:Object,triggerClass:String,triggerStyle:[String,Object],renderIcon:Function},ze=O({name:`Upload`,props:Re,setup(e){e.abstract&&e.listType===`image-card`&&d(`upload`,`when the list-type is image-card, abstract is not supported.`);let{mergedClsPrefixRef:t,inlineThemeDisabled:n,mergedRtlRef:r}=h(e),i=a(`Upload`,`-upload`,Ae,b,e,t),o=v(`Upload`,r,t),s=ne(e),c=M(e.defaultFileList),l=k(e,`fileList`),u=M(null),p={value:!1},m=M(!1),g=new Map,y=oe(l,c),x=N(()=>y.value.map(X)),S=N(()=>{let{max:t}=e;return t!==void 0&&x.value.length>=t});function C(){u.value?.click()}function w(e){let t=e.target;P(t.files?Array.from(t.files).map(e=>({file:e,entry:null,source:`input`})):null,e),t.value=``}function T(t){let{"onUpdate:fileList":n,onUpdateFileList:r}=e;n&&_(n,t),r&&_(r,t),c.value=t}let D=N(()=>e.multiple||e.directory),O=(t,n,r={append:!1,remove:!1})=>{let{append:i,remove:a}=r,o=Array.from(x.value),s=o.findIndex(e=>e.id===t.id);if(i||a||~s){i?o.push(t):a?o.splice(s,1):o.splice(s,1,t);let{onChange:r}=e;r&&r({file:t,fileList:o,event:n}),T(o)}},A=Promise.resolve(),j=(e,t,n)=>{A=A.then(async()=>{await E(),O(e,t,n)})};function P(t,n){if(!t||t.length===0)return;let{onBeforeUpload:r}=e;t=D.value?t:[t[0]];let{max:i,accept:a}=e;t=t.filter(({file:e,source:t})=>t===`dnd`&&a?.trim()?ge(e.name,e.type,a):!0),i&&(t=t.slice(0,i-x.value.length));let o=B();Promise.all(t.map(async({file:e,entry:t})=>{let n={id:B(),batchId:o,name:e.name,status:`pending`,percentage:0,file:e,url:null,type:e.type,thumbnailUrl:null,fullPath:t?.fullPath??`/${e.webkitRelativePath||e.name}`};return!r||await r({file:n,fileList:x.value})!==!1?n:null})).then(async e=>{let t=Promise.resolve();e.forEach(e=>{t=t.then(E).then(()=>{e&&O(e,n,{append:!0})})}),await t}).then(()=>{e.defaultUpload&&F()})}function F({fileId:t,retry:n=!1}={}){let{method:r,action:i,withCredentials:a,headers:o,data:s,name:c}=e,l=t===void 0?x.value:x.value.filter(e=>e.id===t),u=n||t!==void 0;l.forEach(t=>{let{status:n}=t;(n===`pending`||n===`error`&&u)&&(e.customRequest?Ne({inst:{doChange:j,xhrMap:g,onFinish:e.onFinish,onError:e.onError},file:t,action:i,withCredentials:a,headers:o,data:s,customRequest:e.customRequest}):Le({doChange:O,xhrMap:g,onFinish:e.onFinish,onError:e.onError,isErrorState:e.isErrorState},c,t,{method:r,action:i,withCredentials:a,responseType:e.responseType,headers:o,data:s}))})}function I(t){if(t.thumbnailUrl)return t.thumbnailUrl;let{createThumbnailUrl:n}=e;return n?n(t.file,t)??(t.url||``):t.url?t.url:t.file?de(t.file):``}let L=N(()=>{let{common:{cubicBezierEaseInOut:e},self:{draggerColor:t,draggerBorder:n,draggerBorderHover:r,itemColorHover:a,itemColorHoverError:o,itemTextColorError:s,itemTextColorSuccess:c,itemTextColor:l,itemIconColor:u,itemDisabledOpacity:d,lineHeight:f,borderRadius:p,fontSize:m,itemBorderImageCardError:h,itemBorderImageCard:g}}=i.value;return{"--n-bezier":e,"--n-border-radius":p,"--n-dragger-border":n,"--n-dragger-border-hover":r,"--n-dragger-color":t,"--n-font-size":m,"--n-item-color-hover":a,"--n-item-color-hover-error":o,"--n-item-disabled-opacity":d,"--n-item-icon-color":u,"--n-item-text-color":l,"--n-item-text-color-error":s,"--n-item-text-color-success":c,"--n-line-height":f,"--n-item-border-image-card-error":h,"--n-item-border-image-card":g}}),R=n?f(`upload`,void 0,L,e):void 0;ee(W,{mergedClsPrefixRef:t,mergedThemeRef:i,showCancelButtonRef:k(e,`showCancelButton`),showDownloadButtonRef:k(e,`showDownloadButton`),showRemoveButtonRef:k(e,`showRemoveButton`),showRetryButtonRef:k(e,`showRetryButton`),onRemoveRef:k(e,`onRemove`),onDownloadRef:k(e,`onDownload`),customDownloadRef:k(e,`customDownload`),mergedFileListRef:x,triggerClassRef:k(e,`triggerClass`),triggerStyleRef:k(e,`triggerStyle`),shouldUseThumbnailUrlRef:k(e,`shouldUseThumbnailUrl`),renderIconRef:k(e,`renderIcon`),xhrMap:g,submit:F,doChange:O,showPreviewButtonRef:k(e,`showPreviewButton`),alwaysShowActionsRef:k(e,`alwaysShowActions`),onPreviewRef:k(e,`onPreview`),getFileThumbnailUrlResolver:I,listTypeRef:k(e,`listType`),dragOverRef:m,openOpenFileDialog:C,draggerInsideRef:p,handleFileAddition:P,mergedDisabledRef:s.mergedDisabledRef,maxReachedRef:S,fileListClassRef:k(e,`fileListClass`),fileListStyleRef:k(e,`fileListStyle`),abstractRef:k(e,`abstract`),acceptRef:k(e,`accept`),cssVarsRef:n?void 0:L,themeClassRef:R?.themeClass,onRender:R?.onRender,showTriggerRef:k(e,`showTrigger`),imageGroupPropsRef:k(e,`imageGroupProps`),mergedDirectoryDndRef:N(()=>e.directoryDnd??e.directory),onRetryRef:k(e,`onRetry`)});let z={clear:()=>{c.value=[]},submit:F,openOpenFileDialog:C};return{mergedClsPrefix:t,draggerInsideRef:p,rtlEnabled:o,inputElRef:u,mergedTheme:i,dragOver:m,mergedMultiple:D,cssVars:n?void 0:L,themeClass:R?.themeClass,onRender:R?.onRender,handleFileInputChange:w,...z}},render(){let{draggerInsideRef:e,mergedClsPrefix:t,$slots:n,directory:r,onRender:i}=this;n.default&&!this.abstract&&n.default()[0]?.type?.__UPLOAD_DRAGGER__&&(e.value=!0);let a=(A(),F(`input`,R(this.inputProps,{ref:`inputElRef`,type:`file`,class:`${t}-upload-file-input`,accept:this.accept,multiple:this.mergedMultiple,onChange:this.handleFileInputChange,webkitdirectory:r||void 0,directory:r||void 0}),null,16,je));return this.abstract?(A(),F(I,{key:1},[g(()=>n.default?.()),(A(),z(te,{to:`body`},m(a),1024))],64)):(i?.(),A(),F(`div`,{class:s([`${t}-upload`,this.rtlEnabled&&`${t}-upload--rtl`,e.value&&`${t}-upload--dragger-inside`,this.dragOver&&`${t}-upload--drag-over`,this.themeClass]),style:j(this.cssVars)},[g(()=>a),g(()=>this.showTrigger&&this.listType!==`image-card`&&(A(),z(Z,null,m(n),1024))),g(()=>this.showFileList&&(A(),z(ke,null,m(n),1024)))],6))}});export{G as n,ze as t};