package main

// 浏览器补丁：把"不理解 /app/opencode 前缀"的 Web 应用适配到飞牛门户路径下。
// 覆盖 fetch / XHR / WebSocket / Worker / EventSource 的路径改写。
// 注意：本常量必须是**语法正确**的 JS（用 node --check 校验过），否则整个补丁不会执行。

const RUNTIME_SHIM = `(function(){
  try{
    var PB="/app/opencode";
    var P0=location.pathname;
    // 仅门户入口（带前缀）需要补丁；直连入口原样使用
    if(P0.indexOf(PB)!==0)return;
    // SPA 路由按 location.pathname 匹配根路由，这里在应用脚本运行前把地址伪装成 "/"
    // （真实请求路径仍由下方 fetch/XHR 补丁补回前缀，<base href> 保证相对路径正确）
    try{ history.replaceState(null,"","/"); }catch(e){}
    function fix(u){
      try{
        if(u&&typeof u==="object"&&typeof u.href==="string"){u=u.href;}
        if(typeof u!=="string"||!u)return u;
        if(u.indexOf("//")===0)return u;
        if(u.charAt(0)==="/"){
          if(u===PB||u.indexOf(PB+"/")===0)return u;
          return PB+u;
        }
        var i=u.indexOf("://");
        if(i>0){
          var rest=u.slice(i+3), j=rest.indexOf("/");
          var host=j<0?rest:rest.slice(0,j);
          if(host!==location.host)return u;
          var pp=j<0?"/":rest.slice(j);
          if(pp===PB||pp.indexOf(PB+"/")===0)return u;
          return u.slice(0,i+3)+host+PB+pp;
        }
        return u;
      }catch(e){return u;}
    }
    window.__gwFix=fix;
    var _f=window.fetch;
    if(_f){
      window.fetch=function(input,init){
        try{
          if(typeof input==="string"){input=fix(input);}
          else if(input&&typeof input.href==="string"){input=fix(input.href);}
          else if(input&&input.url){var nu=fix(input.url);if(nu!==input.url){input=new Request(nu,input);}}
        }catch(e){}
        return _f.call(this,input,init);
      };
    }
    var _o=XMLHttpRequest.prototype.open;
    XMLHttpRequest.prototype.open=function(m,u){try{arguments[1]=fix(u);}catch(e){}return _o.apply(this,arguments);};
    var _W=window.WebSocket;
    if(_W){
      var NW=function(url,proto){
        try{
          if(url&&typeof url==="object"&&typeof url.href==="string"){url=url.href;}
          if(typeof url==="string"&&(url.indexOf("ws://")===0||url.indexOf("wss://")===0)){
            var p=url.indexOf("://")+3,rest=url.slice(p),sl=rest.indexOf("/");
            url=sl<0?"/":rest.slice(sl);
          }
          url=fix(url);
        }catch(e){}
        return proto===undefined?new _W(url):new _W(url,proto);
      };
      NW.prototype=_W.prototype;
      NW.CONNECTING=_W.CONNECTING;NW.OPEN=_W.OPEN;NW.CLOSING=_W.CLOSING;NW.CLOSED=_W.CLOSED;
      window.WebSocket=NW;
    }
    var _WK=window.Worker;
    if(_WK){
      var NWk=function(u,o){try{u=fix(u);}catch(e){}return new _WK(u,o);};
      NWk.prototype=_WK.prototype;
      window.Worker=NWk;
    }
    var _E=window.EventSource;
    if(_E){
      var NE=function(u,c){try{u=fix(u);}catch(e){}return new _E(u,c);};
      NE.prototype=_E.prototype;
      window.EventSource=NE;
    }
    // Service Worker：兜住动态 import() 等补丁无法覆盖的路径逃逸（scope 限门户前缀）
    if(navigator.serviceWorker&&navigator.serviceWorker.register){
      navigator.serviceWorker.register(PB+"/__gw_sw.js",{scope:PB+"/"}).catch(function(){});
    }

    // 自动打开项目：应用必须先打开项目，右上角「+ 新建会话」才可用。
    // 进入"还没有内容"状态时，自动点「添加项目」→ 按目录名点中弹层里的第一个文件夹。
    var _autoTried=0, _autoName=null;
    function _autoClickName(){
      if(sessionStorage.getItem("oc_auto_proj")==="1")return true;
      var name=_autoName||"我的项目";
      var els=Array.prototype.slice.call(document.querySelectorAll("body *")).filter(function(e){
        return (e.innerText||"").trim()===name && e.getBoundingClientRect().height<44;
      });
      if(!els.length)return false;
      try{ els[els.length-1].click(); }catch(e){}
      setTimeout(function(){
        var t=document.body.innerText||"";
        if(t.indexOf("创建")>=0 && t.indexOf("打开项目")<0) sessionStorage.setItem("oc_auto_proj","1");
      },400);
      return true;
    }
    function _autoOpenProject(){
      try{
        if(sessionStorage.getItem("oc_auto_proj")==="1")return;
        var txt=document.body?document.body.innerText:"";
        if(!txt)return;
        if(txt.indexOf("通用")>=0&&txt.indexOf("提供商")>=0)return;   // 设置弹层开着，别乱点
        var hasDlg=txt.indexOf("打开项目")>=0;
        if(!hasDlg){
          if(txt.indexOf("创建一个会话以开始")<0&&txt.indexOf("还没有内容")<0)return;
          var btn=document.querySelector('[data-action="home-add-project"]');
          if(!btn||_autoTried)return;
          _autoTried=1; btn.click();
          fetch(fix("/find/file?query=&dirs=true&limit=50")).then(function(r){return r.json();}).then(function(arr){
            if(Array.isArray(arr)&&arr.length){
              var it=arr[0];
              _autoName=(typeof it==="string")?it.split("/").filter(Boolean).pop():(it.name||(it.path||"").split("/").filter(Boolean).pop()||null);
            }
          }).catch(function(){});
        }
        var n=0, iv=setInterval(function(){
          n++;
          _autoClickName();
          if(n>=12||sessionStorage.getItem("oc_auto_proj")==="1")clearInterval(iv);
        },800);
      }catch(e){}
    }
    setTimeout(_autoOpenProject,2600);

    // 项目目录提示：常驻显示实际完整路径（审核/用户都能直接看到文件在哪）。
    // 可见=绿色条；不可见=橙色条并给出修复指引。点击右侧 × 关闭。
    setTimeout(function(){
      try{
        fetch(fix("/gateway/status")).then(function(r){return r.json();}).then(function(st){
          if(!st || !st.workdir) return;
          var vis = (st.visible === true);
          var d = document.createElement("div");
          d.style.cssText = "position:fixed;left:0;right:0;top:0;z-index:2147483600;padding:5px 34px 5px 12px;font:12.5px/1.5 system-ui;text-align:left;box-shadow:0 1px 4px rgba(0,0,0,.25)";
          d.style.background = vis ? "#0f5132" : "#7c2d12";
          d.style.color = vis ? "#d1e7dd" : "#fff2e8";
          var x = document.createElement("span");
          x.textContent = "✕";
          x.style.cssText = "position:absolute;right:10px;top:4px;cursor:pointer;font-size:14px";
          x.onclick = function(){ d.remove(); };
          if(vis){
            d.appendChild(document.createTextNode("项目文件目录（完整路径）：" + st.workdir + " —— 可在 文件管理 → 我的文件 中直接查看与编辑"));
          }else{
            d.appendChild(document.createTextNode("项目目录尚未在「我的文件」中（当前：" + st.workdir + "）。修复：文件管理 新建文件夹 → 设置→应用→OpenCode→访问权限 授权（需管理员）→ 重启应用"));
          }
          d.appendChild(x);
          document.body.appendChild(d);
        }).catch(function(){});
      }catch(e){}
    },3000);



    // 新建会话防抖：连点会创建多个会话。2.5 秒内重复点击直接吞掉，并显示轻提示。
    var _lastNewSession = 0;
    function _toast(msg){
      try{
        var id="__oc_toast__";
        var el=document.getElementById(id);
        if(!el){
          el=document.createElement("div");
          el.id=id;
          el.style.cssText="position:fixed;left:50%;bottom:56px;transform:translateX(-50%);background:rgba(20,22,30,.92);color:#e5e7eb;padding:8px 16px;border-radius:8px;font:13px system-ui;z-index:2147483000;pointer-events:none;transition:opacity .3s";
          document.body.appendChild(el);
        }
        el.textContent=msg;
        el.style.opacity="1";
        clearTimeout(el.__t);
        el.__t=setTimeout(function(){ el.style.opacity="0"; },2600);
      }catch(e){}
    }
    document.addEventListener("click",function(ev){
      try{
        var el=ev.target, hit=false;
        for(var i=0;i<6&&el;i++){
          if(el.tagName==="BUTTON"&&(el.getAttribute("aria-label")==="新建会话"||(el.innerText||"").trim()==="新建会话")){hit=true;break;}
          el=el.parentElement;
        }
        if(!hit)return;
        var now=Date.now();
        if(now-_lastNewSession<2500){ ev.stopImmediatePropagation(); ev.preventDefault(); return; }
        _lastNewSession=now;
        _toast("正在新建会话…");
        setTimeout(function(){
          var t=document.body.innerText||"";
          if(t.indexOf("随便问点什么")>=0||t.indexOf("可查看命令")>=0){
            try{var el2=document.getElementById("__oc_toast__");if(el2)el2.style.opacity="0";}catch(e){}
          }
        },1800);
      }catch(e){}
    },true);

    // 预热：提前拉取服务商/智能体列表，缩短首次新建会话的等待
    setTimeout(function(){
      try{ fetch(fix("/provider")).catch(function(){}); fetch(fix("/agent")).catch(function(){}); }catch(e){}
    },1500);

    // 「+ 新建会话」兜底：该按钮在无会话时可能只弹提示不建会话，
    // 点击后 800ms 内未进入输入区则自动点页面中央的「新建会话」按钮。
    document.addEventListener("click",function(ev){
      try{
        var el=ev.target, hit=false;
        for(var i=0;i<6&&el;i++){
          if(el.tagName==="BUTTON"&&el.getAttribute("aria-label")==="新建会话"){hit=true;break;}
          el=el.parentElement;
        }
        if(!hit)return;
        setTimeout(function(){
          var t=document.body.innerText||"";
          if(t.indexOf("随便问点什么")>=0||t.indexOf("可查看命令")>=0)return;
          var bs=document.querySelectorAll("button");
          for(var j=0;j<bs.length;j++){
            if((bs[j].innerText||"").trim()==="新建会话"){bs[j].click();return;}
          }
        },800);
      }catch(e){}
    },true);
  }catch(e){}
})();`

// ShimScript 返回外链补丁脚本内容（外链才能通过页面 CSP 的 script-src 'self'）
func ShimScript() string { return RUNTIME_SHIM }

// injectShim 在 HTML 中注入外链补丁，并把首屏绝对路径改写为带前缀形式。
// <script type="module" src="/assets/..."> 这类资源在补丁执行前就被浏览器拉取，
// 必须在服务端改写，否则 404 白屏。
// 需要补前缀的绝对路径白名单（对齐 androidemu 的 STR_PATHS 思路）
var STR_PATHS = []string{"/assets/", "/api/", "/favicon", "/apple-touch-icon", "/site.webmanifest", "/global/", "/session", "/project", "/config", "/provider", "/find/", "/file", "/pty", "/mcp", "/experimental/", "/skill", "/shell", "/question/", "/permission/", "/integration/", "/model", "/location", "/reference", "/agent", "/command", "/path"}

func injectShim(body []byte) []byte {
	body = rewriteAbsPaths(body, []byte(`="/`), []byte(`="`+PREFIX+`/`))
	body = rewriteAbsPaths(body, []byte(`url(/`), []byte(`url(`+PREFIX+`/`))
	for _, p := range STR_PATHS {
		body = rewriteAbsPaths(body, []byte(`"`+p), []byte(`"`+PREFIX+p))
		body = rewriteAbsPaths(body, []byte(`'`+p), []byte(`'`+PREFIX+p))
	}
	// 固定标题栏：钉在顶部 + 禁页面级滚动（应用为全高布局，视图切换时不再整体上跳）
	// 顶部留白：应用自绘标题栏贴到窗口边角太挤，加 8px 内边距（border-box 防溢出）
	css := []byte(`<style>#root{padding-top:8px!important;box-sizing:border-box!important}[role="dialog"],[data-slot*="dialog"],[data-component*="dialog"],[data-component*="popover"],[data-slot*="popover"],[data-component*="menu"],[data-slot*="menu"]{max-height:92vh!important;overflow-y:auto!important}</style>`)
	tag := []byte(`<base href="` + PREFIX + `/">` + string(css) + `<script src="` + PREFIX + `/__gw_shim.js"></script>`)
	for _, marker := range [][]byte{[]byte("<head>"), []byte("<head "), []byte("<html>"), []byte("<html ")} {
		idx := indexOfCI(body, marker)
		if idx >= 0 {
			pos := idx + len(marker)
			out := make([]byte, 0, len(body)+len(tag))
			out = append(out, body[:pos]...)
			out = append(out, tag...)
			out = append(out, body[pos:]...)
			return out
		}
	}
	out := make([]byte, 0, len(body)+len(tag))
	out = append(out, tag...)
	out = append(out, body...)
	return out
}

// rewriteAbsPaths 把 "/x 形式的绝对路径补上前缀；已带前缀的不重复加
func rewriteAbsPaths(body, from, to []byte) []byte {
	out := make([]byte, 0, len(body)+64)
	i := 0
	for {
		j := indexOfBytes(body[i:], from)
		if j < 0 {
			out = append(out, body[i:]...)
			return out
		}
		j += i
		out = append(out, body[i:j]...)
		out = append(out, to...)
		k := j + len(from)
		if len(body) >= k+len(PREFIX)+1 && string(body[k:k+len(PREFIX)]) == PREFIX && body[k+len(PREFIX)] == '/' {
			k += len(PREFIX) + 1
		}
		i = k
	}
}

func indexOfBytes(h, n []byte) int {
	if len(n) == 0 || len(h) < len(n) {
		return -1
	}
	for i := 0; i+len(n) <= len(h); i++ {
		match := true
		for j := range n {
			if h[i+j] != n[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func indexOfCI(haystack, needle []byte) int {
	n := len(needle)
	if n == 0 || len(haystack) < n {
		return -1
	}
	limit := len(haystack) - n
	for i := 0; i <= limit; i++ {
		match := true
		for j := 0; j < n; j++ {
			a := haystack[i+j]
			b := needle[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// GW_SW：Service Worker——接管受控页面发出的所有请求，把逃逸到根路径的应用请求补回前缀。
// 动态 import() 无法在页面侧打补丁，只有 SW 能拦截。
const GW_SW = `const PB="/app/opencode";
self.addEventListener("install",function(e){self.skipWaiting();});
self.addEventListener("activate",function(e){e.clients.claim();});
self.addEventListener("fetch",function(e){
  try{
    var url=new URL(e.request.url);
    if(url.origin!==self.location.origin)return;
    if(url.pathname.indexOf(PB)===0)return;
    // 受控页面发出的所有根路径请求都是应用的（iframe 内不会请求飞牛自身资源）
    if(e.request.mode==="navigate"&&url.pathname!=="/")return;
    if(e.request.mode==="navigate"&&url.pathname==="/"){e.respondWith(fetch(PB+"/"));return;}
    e.respondWith(fetch(new Request(PB+url.pathname+url.search,e.request)));
  }catch(err){}
});`
