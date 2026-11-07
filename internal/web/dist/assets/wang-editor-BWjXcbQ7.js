import{b as e}from"./user-D2XtQlil.js";import{r as t,t as n}from"./Tabs-B9fBMyNB.js";import{Et as r,S as i,V as a,_t as o,at as s,b as c,dt as l,f as u,h as d,y as f}from"./runtime-core.esm-bundler-B5qSvmKr.js";import{t as p}from"./Button-DUNxlS0h.js";import{t as m}from"./Input-ByrqO_2o.js";import{t as h}from"./Card-D_p3JLbh.js";import{n as g,t as _}from"./CollapseItem-Kh2AvWLD.js";import{t as v}from"./_plugin-vue_export-helper-BDNMzG2s.js";import{t as y}from"./art-row-CNYLSOAV.js";import{t as b}from"./art-col-DgM3y2A4.js";import{t as x}from"./art-wang-editor-DK2CqXqi.js";var S={class:`page-content mb-5`},C={class:`card-header`},w={class:`header-buttons`},T={class:`card-header`},E={class:`header-buttons`},D=[`innerHTML`],O=[`innerHTML`],k={class:`toolbar-explanation`},A=v(i({name:`WidgetsWangEditor`,__name:`index`,setup(i){let v=o(),A=o(),j=o(`preview`),M=o(`preview`),N=o([`basic`]),P=[`bold`,`italic`,`underline`,`|`,`bulletedList`,`numberedList`,`|`,`insertLink`,`insertImage`,`|`,`undo`,`redo`],F=o(`<h1>🎨 完整工具栏编辑器示例</h1>
<p>这个编辑器包含所有功能，您可以体验丰富的格式编辑功能。</p>

<h2>✨ 文本样式</h2>
<p><strong>这是加粗的文字</strong></p>
<p><em>这是斜体文字</em></p>
<p><u>这是下划线文字</u></p>
<p><span style="color: rgb(194, 79, 74);">这是彩色文字</span></p>

<h2>📝 列表和待办</h2>
<ul>
  <li>无序列表项 1</li>
  <li>无序列表项 2</li>
</ul>

<ol>
  <li>有序列表项 1</li>
  <li>有序列表项 2</li>
</ol>

<ul class="w-e-todo">
  <li class="w-e-todo-item"><input type="checkbox" checked="true" readonly="true" disabled="disabled"><span>已完成的任务</span></li>
  <li class="w-e-todo-item"><input type="checkbox" readonly="true" disabled="disabled"><span>待完成的任务</span></li>
</ul>

<h2>💬 引用和表格</h2>
<blockquote>
  这是一段引用文字，展示引用格式的效果。
</blockquote>

<table style="border-collapse: collapse; width: 100%;" border="1">
  <thead>
    <tr><th>功能</th><th>描述</th></tr>
  </thead>
  <tbody>
    <tr><td>完整工具栏</td><td>包含所有编辑功能</td></tr>
    <tr><td>自定义配置</td><td>支持灵活的工具栏配置</td></tr>
  </tbody>
</table>

<h2>💻 代码块</h2>
<pre><code class="language-javascript">// 完整编辑器支持代码高亮
function createEditor() {
  return new WangEditor({
    container: '#editor',
    toolbar: 'full' // 完整工具栏
  });
}</code></pre>

<p>🔗 <a href="https://www.wangeditor.com/" target="_blank">访问官网了解更多</a></p>`),I=o(`<h1>✨ 简化工具栏编辑器示例</h1>
<p>这个编辑器只包含基础的编辑功能，界面更加简洁。</p>

<h2>基础文本格式</h2>
<p><strong>加粗文字</strong></p>
<p><em>斜体文字</em></p>
<p><u>下划线文字</u></p>

<h2>列表功能</h2>
<ul>
  <li>无序列表项 1</li>
  <li>无序列表项 2</li>
</ul>

<ol>
  <li>有序列表项 1</li>
  <li>有序列表项 2</li>
</ol>

<h2>链接和图片</h2>
<p>支持插入 <a href="https://www.wangeditor.com/" target="_blank">链接</a> 和图片。</p>

<p>简化版编辑器专注于基础功能，适合简单的内容编辑需求。</p>`),L=()=>{v.value?.clear(),e.success(`完整编辑器已清空`)},R=()=>{let t=v.value?.getHtml();console.log(`完整编辑器内容:`,t),e.success(`完整编辑器内容已输出到控制台`)},z=()=>{v.value?.setHtml(`<h2>🎉 完整编辑器演示内容</h2>
<p>这是通过方法设置的演示内容，展示完整编辑器的强大功能。</p>
<ul>
  <li>支持丰富的文本格式</li>
  <li>包含表格、代码块等高级功能</li>
  <li>提供完整的编辑体验</li>
</ul>
<table style="border-collapse: collapse; width: 100%;" border="1">
  <tr><th>特性</th><th>状态</th></tr>
  <tr><td>完整工具栏</td><td>✅ 已启用</td></tr>
  <tr><td>高级功能</td><td>✅ 已启用</td></tr>
</table>`),e.success(`已设置完整编辑器演示内容`)},B=()=>{A.value?.clear(),e.success(`简化编辑器已清空`)},V=()=>{let t=A.value?.getHtml();console.log(`简化编辑器内容:`,t),e.success(`简化编辑器内容已输出到控制台`)},H=()=>{A.value?.setHtml(`<h2>⚡ 简化编辑器演示内容</h2>
<p>这是通过方法设置的演示内容，展示简化编辑器的核心功能。</p>
<ul>
  <li><strong>基础格式</strong>：加粗、斜体、下划线</li>
  <li><em>列表支持</em>：有序和无序列表</li>
  <li><u>媒体插入</u>：链接和图片</li>
</ul>
<ol>
  <li>界面简洁清爽</li>
  <li>功能专注实用</li>
  <li>适合快速编辑</li>
</ol>
<p>🔗 <a href="https://example.com" target="_blank">这是一个链接示例</a></p>`),e.success(`已设置简化编辑器演示内容`)};return(e,i)=>{let o=p,U=x,W=h,G=t,K=m,q=n,J=b,Y=y,X=_,Z=g;return a(),d(`div`,S,[c(W,{class:`editor-card`},{header:s(()=>[u(`div`,C,[i[10]||(i[10]=u(`span`,null,`🛠️ 完整工具栏编辑器`,-1)),u(`div`,w,[c(o,{size:`small`,onClick:L},{default:s(()=>[...i[7]||(i[7]=[f(`清空`,-1)])]),_:1}),c(o,{size:`small`,onClick:R},{default:s(()=>[...i[8]||(i[8]=[f(`获取内容`,-1)])]),_:1}),c(o,{size:`small`,onClick:z},{default:s(()=>[...i[9]||(i[9]=[f(`设置示例`,-1)])]),_:1})])])]),default:s(()=>[c(U,{ref_key:`fullEditorRef`,ref:v,modelValue:r(F),"onUpdate:modelValue":i[0]||(i[0]=e=>l(F)?F.value=e:null),height:`400px`,placeholder:`请输入内容，体验完整的编辑功能...`,"exclude-keys":[]},null,8,[`modelValue`])]),_:1}),c(W,{class:`editor-card`},{header:s(()=>[u(`div`,T,[i[14]||(i[14]=u(`span`,null,`✨ 简化工具栏编辑器`,-1)),u(`div`,E,[c(o,{size:`small`,onClick:B},{default:s(()=>[...i[11]||(i[11]=[f(`清空`,-1)])]),_:1}),c(o,{size:`small`,onClick:V},{default:s(()=>[...i[12]||(i[12]=[f(`获取内容`,-1)])]),_:1}),c(o,{size:`small`,onClick:H},{default:s(()=>[...i[13]||(i[13]=[f(`设置示例`,-1)])]),_:1})])])]),default:s(()=>[c(U,{ref_key:`simpleEditorRef`,ref:A,modelValue:r(I),"onUpdate:modelValue":i[1]||(i[1]=e=>l(I)?I.value=e:null),height:`400px`,placeholder:`请输入内容，体验简化的编辑功能...`,"toolbar-keys":P},null,8,[`modelValue`])]),_:1}),c(W,{class:`preview-card`},{header:s(()=>[...i[15]||(i[15]=[u(`span`,null,`📖 内容预览对比`,-1)])]),default:s(()=>[c(Y,{gutter:20},{default:s(()=>[c(J,{span:12},{default:s(()=>[i[16]||(i[16]=u(`h3`,null,`完整编辑器内容`,-1)),c(q,{value:r(j),"onUpdate:value":i[3]||(i[3]=e=>l(j)?j.value=e:null)},{default:s(()=>[c(G,{tab:`渲染效果`,name:`preview`},{default:s(()=>[u(`div`,{class:`content-preview`,innerHTML:r(F)},null,8,D)]),_:1}),c(G,{tab:`HTML源码`,name:`html`},{default:s(()=>[c(K,{value:r(F),"onUpdate:value":i[2]||(i[2]=e=>l(F)?F.value=e:null),type:`textarea`,rows:8,placeholder:`HTML源码`,readonly:``},null,8,[`value`])]),_:1})]),_:1},8,[`value`])]),_:1}),c(J,{span:12},{default:s(()=>[i[17]||(i[17]=u(`h3`,null,`简化编辑器内容`,-1)),c(q,{value:r(M),"onUpdate:value":i[5]||(i[5]=e=>l(M)?M.value=e:null)},{default:s(()=>[c(G,{tab:`渲染效果`,name:`preview`},{default:s(()=>[u(`div`,{class:`content-preview`,innerHTML:r(I)},null,8,O)]),_:1}),c(G,{tab:`HTML源码`,name:`html`},{default:s(()=>[c(K,{value:r(I),"onUpdate:value":i[4]||(i[4]=e=>l(I)?I.value=e:null),type:`textarea`,rows:8,placeholder:`HTML源码`,readonly:``},null,8,[`value`])]),_:1})]),_:1},8,[`value`])]),_:1})]),_:1})]),_:1}),c(W,{class:`usage-card`},{header:s(()=>[...i[18]||(i[18]=[u(`span`,null,`📚 使用说明`,-1)])]),default:s(()=>[c(Z,{"expanded-names":r(N),"onUpdate:expandedNames":i[6]||(i[6]=e=>l(N)?N.value=e:null)},{default:s(()=>[c(X,{title:`基础用法`,name:`basic`},{default:s(()=>[...i[19]||(i[19]=[u(`pre`,null,[u(`code`,{class:`language-vue`},`<template>
          <ArtWangEditor v-model="content" />
          </template>

          <script setup lang="ts">
          import { ref } from 'vue'

          const content = ref('<p>初始内容</p>')
          <\/script>`)],-1)])]),_:1}),c(X,{title:`完整工具栏配置`,name:`full`},{default:s(()=>[...i[20]||(i[20]=[u(`pre`,null,[u(`code`,{class:`language-vue`},`<template>
          <!-- 显示所有工具，不排除任何功能 -->
          <ArtWangEditor
          v-model="content"
          :exclude-keys="[]"
          />
          </template>`)],-1)])]),_:1}),c(X,{title:`简化工具栏配置`,name:`simple`},{default:s(()=>[...i[21]||(i[21]=[u(`pre`,null,[u(`code`,{class:`language-vue`},`<template>
          <!-- 只显示基础编辑工具 -->
          <ArtWangEditor
          v-model="content"
          :toolbar-keys="[
          'bold', 'italic', 'underline', '|',
          'bulletedList', 'numberedList', '|',
          'insertLink', 'insertImage', '|',
          'undo', 'redo'
          ]"
          />
          </template>`)],-1)])]),_:1}),c(X,{title:`自定义配置`,name:`config`},{default:s(()=>[...i[22]||(i[22]=[u(`pre`,null,[u(`code`,{class:`language-vue`},`<template>
          <ArtWangEditor
          v-model="content"
          height="600px"
          placeholder="请输入您的内容..."
          :exclude-keys="['fontFamily', 'fontSize']"
          :upload-config="{
          maxFileSize: 5 * 1024 * 1024,
          maxNumberOfFiles: 5
          }"
          />
          </template>`)],-1)])]),_:1}),c(X,{title:`组件方法调用`,name:`methods`},{default:s(()=>[...i[23]||(i[23]=[u(`pre`,null,[u(`code`,{class:`language-vue`},`<template>
          <ArtWangEditor ref="editorRef" v-model="content" />
          <n-button @click="handleClear">清空</n-button>
          <n-button @click="handleFocus">聚焦</n-button>
          <n-button @click="handleGetContent">获取内容</n-button>
          </template>

          <script setup lang="ts">
          import { ref } from 'vue'

          const editorRef = ref()
          const content = ref('')

          const handleClear = () => {
          editorRef.value?.clear()
          }

          const handleFocus = () => {
          editorRef.value?.focus()
          }

          const handleGetContent = () => {
          const html = editorRef.value?.getHtml()
          console.log('编辑器内容:', html)
          }
          <\/script>`)],-1)])]),_:1}),c(X,{title:`工具栏配置说明`,name:`toolbar-config`},{default:s(()=>[u(`div`,k,[i[26]||(i[26]=u(`h4`,null,`完整工具栏 vs 简化工具栏`,-1)),c(Y,{gutter:16},{default:s(()=>[c(J,{span:12},{default:s(()=>[...i[24]||(i[24]=[u(`h5`,null,`✅ 完整工具栏包含：`,-1),u(`ul`,null,[u(`li`,null,`文本格式：加粗、斜体、下划线、字体颜色、背景色`),u(`li`,null,`段落格式：标题、引用、对齐方式、缩进`),u(`li`,null,`列表：有序列表、无序列表、待办事项`),u(`li`,null,`插入：链接、图片、表格、分割线、表情`),u(`li`,null,`代码：代码块、行内代码`),u(`li`,null,`操作：撤销、重做、全屏、清除格式`)],-1)])]),_:1}),c(J,{span:12},{default:s(()=>[...i[25]||(i[25]=[u(`h5`,null,`⚡ 简化工具栏包含：`,-1),u(`ul`,null,[u(`li`,null,`基础格式：加粗、斜体、下划线`),u(`li`,null,`列表：有序列表、无序列表`),u(`li`,null,`插入：链接、图片`),u(`li`,null,`操作：撤销、重做`)],-1),u(`p`,{class:`note`},`适用于简单的文本编辑场景，界面更清爽。`,-1)])]),_:1})]),_:1})])]),_:1})]),_:1},8,[`expanded-names`])]),_:1})])}}}),[[`__scopeId`,`data-v-bbb5b0de`]]);export{A as default};