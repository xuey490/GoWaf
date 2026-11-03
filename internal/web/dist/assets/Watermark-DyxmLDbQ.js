import{at as e,c as t,ct as n,g as r,it as i,j as a,lt as o,s,u as c,w as l,y as u}from"./resolve-slot-D0ESJhJD.js";import{S as d,V as f,Wt as p,_t as m,h,it as g}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{t as _}from"./on-fonts-ready-Clf4bkGj.js";import{t as v}from"./is-browser-e-NGrvPW.js";var y=s({name:`Watermark`,common:c,self(e){let{fontFamily:t}=e;return{fontFamily:t}}}),b=i([e(`watermark-container`,`
 position: relative;
 `,[o(`selectable`,`
 user-select: none;
 -webkit-user-select: none;
 `),n(`global-rotate`,`
 overflow: hidden;
 `),n(`fullscreen`,`
 top: 0;
 left: 0;
 width: 100%;
 height: 100%;
 pointer-events: none;
 position: fixed;
 `)]),e(`watermark`,`
 position: absolute;
 top: 0;
 left: 0;
 width: 100%;
 height: 100%;
 pointer-events: none;
 background-repeat: repeat;
 `,[n(`fullscreen`,`
 position: fixed;
 `),n(`global-rotate`,`
 position: absolute;
 height: max(284vh, 284vw);
 width: max(284vh, 284vw);
 `)])]);function x(e){if(!e)return 1;let t=e.backingStorePixelRatio||e.webkitBackingStorePixelRatio||e.mozBackingStorePixelRatio||e.msBackingStorePixelRatio||e.oBackingStorePixelRatio||e.backingStorePixelRatio||1;return(window.devicePixelRatio||1)/t}var S={...t.props,debug:Boolean,cross:Boolean,fullscreen:Boolean,width:{type:Number,default:32},height:{type:Number,default:32},zIndex:{type:Number,default:10},xGap:{type:Number,default:0},yGap:{type:Number,default:0},yOffset:{type:Number,default:0},xOffset:{type:Number,default:0},rotate:{type:Number,default:0},textAlign:{type:String,default:`left`},image:String,imageOpacity:{type:Number,default:1},imageHeight:Number,imageWidth:Number,content:String,selectable:{type:Boolean,default:!0},fontSize:{type:Number,default:14},fontFamily:String,fontStyle:{type:String,default:`normal`},fontVariant:{type:String,default:``},fontWeight:{type:Number,default:400},fontColor:{type:String,default:`rgba(128, 128, 128, .3)`},fontStretch:{type:String,default:``},lineHeight:{type:Number,default:14},globalRotate:{type:Number,default:0}},C=d({name:`Watermark`,props:S,setup(e,{slots:n}){let{mergedClsPrefixRef:i}=l(e),o=t(`Watermark`,`-watermark`,b,y,e,i),s=m(``),c=v?document.createElement(`canvas`):null,d=c?c.getContext(`2d`):null,S=m(!1);return _(()=>S.value=!0),g(()=>{if(!c)return;S.value;let t=x(d),{xGap:n,yGap:r,width:i,height:l,yOffset:u,xOffset:f,rotate:p,image:m,content:h,fontColor:g,fontStyle:_,fontVariant:v,fontStretch:y,fontWeight:b,fontFamily:C,fontSize:w,lineHeight:T,debug:E}=e,D=(n+i)*t,O=(r+l)*t,k=f*t,A=u*t;if(c.width=D,c.height=O,d){d.translate(0,0);let n=i*t,r=l*t;if(E&&(d.strokeStyle=`grey`,d.strokeRect(0,0,n,r)),d.rotate(Math.PI/180*p),m){let n=new Image;n.crossOrigin=`anonymous`,n.referrerPolicy=`no-referrer`,n.src=m,n.onload=()=>{d.globalAlpha=e.imageOpacity;let{imageWidth:r,imageHeight:i}=e;d.drawImage(n,k,A,(e.imageWidth||(i?n.width*i/n.height:n.width))*t,(e.imageHeight||(r?n.height*r/n.width:n.height))*t),s.value=c.toDataURL()}}else if(h){E&&(d.strokeStyle=`green`,d.strokeRect(0,0,n,r)),d.font=`${_} ${v} ${b} ${y} ${w*t}px/${T*t}px ${C||o.value.self.fontFamily}`,d.fillStyle=g;let i=0,{textAlign:a}=e;h.split(`
`).map(e=>{let t=d.measureText(e).width;return i=Math.max(i,t),{width:t,line:e}}).forEach(({line:e,width:n},r)=>{let o=a===`left`?0:a===`center`?(i-n)/2:i-n;d.fillText(e,k+o,A+T*t*(r+1))}),s.value=c.toDataURL()}else h||(d.clearRect(0,0,c.width,c.height),s.value=c.toDataURL())}else a(`watermark`,`Canvas is not supported in the browser.`)}),()=>{let{globalRotate:t,fullscreen:a,zIndex:o}=e,c=i.value,l=t!==0&&a,d=`max(142vh, 142vw)`,m=(f(),h(`div`,{class:r([`${c}-watermark`,t!==0&&`${c}-watermark--global-rotate`,a&&`${c}-watermark--fullscreen`]),style:p({transform:t?`translateX(-50%) translateY(-50%) rotate(${t}deg)`:void 0,zIndex:l?void 0:o,backgroundSize:`${e.xGap+e.width}px`,backgroundPosition:t===0?e.cross?`${e.width/2}px ${e.height/2}px, 0 0`:``:e.cross?`calc(${d} + ${e.width/2}px) calc(${d} + ${e.height/2}px), ${d} ${d}`:d,backgroundImage:e.cross?`url(${s.value}), url(${s.value})`:`url(${s.value})`})},null,6));return e.fullscreen&&!t?m:(f(),h(`div`,{class:r([`${c}-watermark-container`,t!==0&&`${c}-watermark-container--global-rotate`,a&&`${c}-watermark-container--fullscreen`,e.selectable&&`${c}-watermark-container--selectable`]),style:p({zIndex:l?o:void 0})},[u(()=>n.default?.()),u(()=>m)],6))}}});export{C as t};