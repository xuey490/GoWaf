import{D as e,a as t,at as n,c as r,dt as i,ft as a,g as o,it as s,l as c,r as l,st as u,ut as d,w as f,y as p}from"./resolve-slot-D0ESJhJD.js";import{Dt as m}from"./user-B1sGzif1.js";import{r as h}from"./Scrollbar-D9NqNphl.js";import{E as g,F as _,O as v,R as y,S as b,V as x,Wt as S,_t as C,d as w,h as T,it as E,p as D,rt as O}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{t as k}from"./color-to-class-G4jmrDIg.js";import{n as A}from"./Tag-CfwO544n.js";import{n as j,t as M}from"./utils-Mo6fqr7w.js";var N=e(`n-avatar-group`),P=n(`avatar`,`
 width: var(--n-merged-size);
 height: var(--n-merged-size);
 color: #FFF;
 font-size: var(--n-font-size);
 display: inline-flex;
 position: relative;
 overflow: hidden;
 text-align: center;
 border: var(--n-border);
 border-radius: var(--n-border-radius);
 --n-merged-color: var(--n-color);
 background-color: var(--n-merged-color);
 transition:
 border-color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 color .3s var(--n-bezier);
`,[i(s(`&`,`--n-merged-color: var(--n-color-modal);`)),a(s(`&`,`--n-merged-color: var(--n-color-popover);`)),s(`img`,`
 width: 100%;
 height: 100%;
 `),u(`text`,`
 white-space: nowrap;
 display: inline-block;
 position: absolute;
 left: 50%;
 top: 50%;
 `),n(`icon`,`
 vertical-align: bottom;
 font-size: calc(var(--n-merged-size) - 6px);
 `),u(`text`,`line-height: 1.25`)]),F=[`src`],I={...r.props,size:[String,Number],src:String,circle:{type:Boolean,default:void 0},objectFit:String,round:{type:Boolean,default:void 0},bordered:{type:Boolean,default:void 0},onError:Function,fallbackSrc:String,intersectionObserverOptions:Object,lazy:Boolean,onLoad:Function,renderPlaceholder:Function,renderFallback:Function,imgProps:Object,color:String},L=b({name:`Avatar`,props:I,slots:Object,setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:n}=f(e),i=C(!1),a=null,o=C(null),s=C(null),l=()=>{let{value:e}=o;if(e&&(a===null||a!==e.innerHTML)){a=e.innerHTML;let{value:t}=s;if(t){let{offsetWidth:n,offsetHeight:r}=t,{offsetWidth:i,offsetHeight:a}=e,o=.9,s=Math.min(n/i*o,r/a*o,1);e.style.transform=`translateX(-50%) translateY(-50%) scale(${s})`}}},u=v(N,null),p=w(()=>{let{size:t}=e;if(t)return t;let{size:n}=u||{};return n||`medium`}),h=r(`Avatar`,`-avatar`,P,m,e,t),g=v(A,null),b=w(()=>{if(u)return!0;let{round:t,circle:n}=e;return t!==void 0||n!==void 0?t||n:g?g.roundRef.value:!1}),x=w(()=>u?!0:e.bordered||!1),S=w(()=>{let t=p.value,n=b.value,r=x.value,{color:i}=e,{self:{borderRadius:a,fontSize:o,color:s,border:c,colorModal:l,colorPopover:u},common:{cubicBezierEaseInOut:f}}=h.value,m;return m=typeof t==`number`?`${t}px`:h.value.self[d(`height`,t)],{"--n-font-size":o,"--n-border":r?c:`none`,"--n-border-radius":n?`50%`:a,"--n-color":i||s,"--n-color-modal":i||l,"--n-color-popover":i||u,"--n-bezier":f,"--n-merged-size":`var(--n-avatar-size-override, ${m})`}}),T=n?c(`avatar`,w(()=>{let t=p.value,n=b.value,r=x.value,{color:i}=e,a=``;return t&&(a+=typeof t==`number`?`a${t}`:t[0]),n&&(a+=`b`),r&&(a+=`c`),i&&(a+=k(i)),a}),S,e):void 0,D=C(!e.lazy);y(()=>{if(e.lazy&&e.intersectionObserverOptions){let t,n=E(()=>{t?.(),t=void 0,e.lazy&&(t=M(s.value,e.intersectionObserverOptions,D))});_(()=>{n(),t?.()})}}),O(()=>e.src||e.imgProps?.src,()=>{i.value=!1});let j=C(!e.lazy);return{textRef:o,selfRef:s,mergedRoundRef:b,mergedClsPrefix:t,fitTextTransform:l,cssVars:n?void 0:S,themeClass:T?.themeClass,onRender:T?.onRender,hasLoadError:i,shouldStartLoading:D,loaded:j,mergedOnError:t=>{if(!D.value)return;i.value=!0;let{onError:n,imgProps:{onError:r}={}}=e;n?.(t),r?.(t)},mergedOnLoad:t=>{let{onLoad:n,imgProps:{onLoad:r}={}}=e;n?.(t),r?.(t),j.value=!0}}},render(){let{$slots:e,src:n,mergedClsPrefix:r,lazy:i,onRender:a,loaded:s,hasLoadError:c,imgProps:u={}}=this;a?.();let d,f=!s&&!c&&(this.renderPlaceholder?this.renderPlaceholder():this.$slots.placeholder?.());return d=this.hasLoadError?this.renderFallback?this.renderFallback():l(e.fallback,()=>[(x(),T(`img`,{src:this.fallbackSrc,style:S({objectFit:this.objectFit})},null,12,F))]):t(e.default,e=>{if(e)return x(),D(h,{key:1,onResize:this.fitTextTransform},{default:()=>(x(),T(`span`,{ref:`textRef`,class:o(`${r}-avatar__text`)},[p(()=>e)],2))},1032,[`onResize`]);if(n||u.src){let e=this.src||u.src;return g(`img`,{...u,loading:j&&!this.intersectionObserverOptions&&i?`lazy`:`eager`,src:i&&this.intersectionObserverOptions?this.shouldStartLoading?e:void 0:e,"data-image-src":e,onLoad:this.mergedOnLoad,onError:this.mergedOnError,style:[u.style||``,{objectFit:this.objectFit},f?{height:`0`,width:`0`,visibility:`hidden`,position:`absolute`}:``]})}}),x(),T(`span`,{ref:`selfRef`,class:o([`${r}-avatar`,this.themeClass]),style:S(this.cssVars)},[p(()=>d),p(()=>i&&f)],6)}});export{L as t};