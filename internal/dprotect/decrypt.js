// GoWAF 动态防护·浏览器端解密器。
//
// 服务端把 HTML 文本节点替换为 <span class="wafx">BASE64(密文)</span>，
// 并在页面尾部注入本脚本。脚本职责：
//   1. 从注入参数还原密钥（按字符码偏移混淆存储）；
//   2. RC4-drop256 生成密钥流，与 Go 侧 internal/dprotect/cipher.go 逐字节一致；
//   3. 解出全部密文，把占位 span 替换回明文文本节点。
//
// 对抗定位：不执行 JS 的采集器拿到的是乱码。这是提高爬取成本的防护层，
// 不是机密性保障（密钥随页面下发）。
(function () {
  'use strict';
  // 页面里已注入的参数：__wafxK 为混淆后的密钥字节数组，__wafxN 未使用（保留扩展）。
  if (!window.__wafxK || !window.__wafxK.length) return;

  // 还原密钥：服务端按 (byte ^ 0x5A ^ idx&7) 存储字节，这里逆运算
  var key = [];
  for (var ki = 0; ki < window.__wafxK.length; ki++) {
    key.push(window.__wafxK[ki] ^ 0x5A ^ (ki & 7));
  }

  // RC4-drop256：KSA -> 丢弃 256 字节 -> 输出 n 字节。与 Go 侧实现对齐。
  function keystream(keyBytes, n) {
    var s = [], i, j = 0, t, out = [], pos;
    for (i = 0; i < 256; i++) s[i] = i;
    for (i = 0; i < 256; i++) {
      j = (j + s[i] + keyBytes[i % keyBytes.length]) & 255;
      t = s[i]; s[i] = s[j]; s[j] = t;
    }
    i = 0; j = 0;
    for (pos = 0; pos < 256 + n; pos++) {
      i = (i + 1) & 255;
      j = (j + s[i]) & 255;
      t = s[i]; s[i] = s[j]; s[j] = t;
      if (pos >= 256) out.push(s[(s[i] + s[j]) & 255]);
    }
    return out;
  }

  // UTF-8 字节数组转 JS 字符串：优先 TextDecoder，环境不支持时退到手写解码。
  function utf8(bytes) {
    if (window.TextDecoder) {
      try { return new window.TextDecoder('utf-8').decode(new Uint8Array(bytes)); } catch (e) { /* 回退 */ }
    }
    var out = '', i = 0, c, c2, c3;
    while (i < bytes.length) {
      c = bytes[i];
      if (c < 128) { out += String.fromCharCode(c); i++; }
      else if (c < 224) { c2 = bytes[i + 1]; out += String.fromCharCode(((c & 31) << 6) | (c2 & 63)); i += 2; }
      else { c2 = bytes[i + 1]; c3 = bytes[i + 2]; out += String.fromCharCode(((c & 15) << 12) | ((c2 & 63) << 6) | (c3 & 63)); i += 3; }
    }
    return out;
  }

  function decryptNode(el) {
    var b64 = el.textContent;
    if (!b64) { el.parentNode && el.parentNode.removeChild(el); return; }
    var raw, i, bytes = [], ks;
    try { raw = window.atob(b64); } catch (e) { el.parentNode && el.parentNode.removeChild(el); return; }
    ks = keystream(key, raw.length);
    for (i = 0; i < raw.length; i++) {
      bytes.push(raw.charCodeAt(i) ^ ks[i]);
    }
    var text = document.createTextNode(utf8(bytes));
    el.parentNode.replaceChild(text, el);
  }

  function run() {
    var nodes = document.querySelectorAll('span.wafx');
    for (var i = 0; i < nodes.length; i++) decryptNode(nodes[i]);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', run);
  } else {
    run();
  }
})();
