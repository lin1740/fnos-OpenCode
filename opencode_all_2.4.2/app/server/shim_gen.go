package main

// 浏览器补丁：把"不理解 /app/opencode 前缀"的 Web 应用适配到飞牛门户路径下。
// 覆盖 fetch / XHR / WebSocket / Worker / EventSource 的路径改写。
// 注意：本常量必须是**语法正确**的 JS（用 node --check 校验过），否则整个补丁不会执行。

const RUNTIME_SHIM = `(function(){
  try{
    // 旧版 WebView 兼容：2.2.6 前端使用了 2023 年后才普及的 JS API，鸿蒙 ArkWeb /
    // iOS WKWebView / 安卓 System WebView 多数不提供。缺失时 health 探测与列表渲染
    // 会直接抛 TypeError，前端回退成「Can't reach this server」。只在缺失时补，
    // 新浏览器全程 no-op。这段必须早于所有应用脚本，且本身只用 ES5 语法。
    (function(){
      function def(o,k,v){
        try{
          if(o && typeof o[k]==="undefined")
            Object.defineProperty(o,k,{value:v,writable:true,configurable:true});
        }catch(e){ try{ if(o && !o[k]) o[k]=v; }catch(e2){} }
      }
      def(Promise,"try",function(fn){
        var a=Array.prototype.slice.call(arguments,1), self=this;
        return new Promise(function(res,rej){
          var v; try{ v=fn.apply(self,a); }catch(e){ rej(e); return; } res(v);
        });
      });
      def(Promise,"withResolvers",function(){
        var res,rej,p=new Promise(function(a,b){res=a;rej=b;});
        return {promise:p,resolve:res,reject:rej};
      });
      def(Map,"groupBy",function(items,fn){
        var out=new Map(), arr=Array.prototype.slice.call(items), i,k;
        for(i=0;i<arr.length;i++){ k=fn(arr[i],i); if(!out.has(k))out.set(k,[]); out.get(k).push(arr[i]); }
        return out;
      });
      def(Object,"groupBy",function(items,fn){
        var out=Object.create(null), arr=Array.prototype.slice.call(items), i,k;
        for(i=0;i<arr.length;i++){ k=fn(arr[i],i); if(!out[k])out[k]=[]; out[k].push(arr[i]); }
        return out;
      });
      def(Object,"hasOwn",function(o,k){ return Object.prototype.hasOwnProperty.call(o,k); });
      def(URL,"canParse",function(u,b){ try{ new URL(u,b); return true; }catch(e){ return false; } });
      def(Array.prototype,"toSorted",function(cmp){ return this.slice().sort(cmp); });
      def(Array.prototype,"toReversed",function(){ return this.slice().reverse(); });
      def(Array.prototype,"toSpliced",function(){
        var a=this.slice(), args=Array.prototype.slice.call(arguments);
        Array.prototype.splice.apply(a,args); return a;
      });
      def(Array.prototype,"with",function(i,v){
        var a=this.slice(), k=i<0?this.length+i:i;
        if(k<0||k>=this.length)throw new RangeError("Invalid index");
        a[k]=v; return a;
      });
      if(typeof AbortSignal!=="undefined" && typeof AbortController!=="undefined"){
        def(AbortSignal,"timeout",function(ms){
          var c=new AbortController();
          setTimeout(function(){ try{ c.abort(new DOMException("Timeout","TimeoutError")); }catch(e){ c.abort(); } },ms);
          return c.signal;
        });
      }
    })();

    var PB="/app/opencode";
    // 诊断信标：手机端白屏时把现场（脚本错误 / 首屏渲染状态）回传给网关写日志。
    // 只在出错或 7 秒后各发一次，正常使用只多一个极小请求，不影响功能与安全。
    (function(){
      var sent=0;
      function rep(kind,msg){
        try{
          if(sent>8)return; sent++;
          var u=PB+"/gateway/jserr?k="+encodeURIComponent(kind)+"&m="+encodeURIComponent(String(msg||"").slice(0,400));
          if(navigator.sendBeacon){ navigator.sendBeacon(u,""); }
          else{ var x=new XMLHttpRequest(); x.open("POST",u,true); x.send(""); }
        }catch(e){}
      }
      try{
        window.addEventListener("error",function(e){
          var m=(e&&e.message||"err")+" @"+(e&&e.filename||"")+":"+(e&&e.lineno||0);
          if(e&&e.error&&e.error.stack)m+=" "+String(e.error.stack).slice(0,300);
          rep("error",m);
        });
        window.addEventListener("unhandledrejection",function(e){
          var r=e&&e.reason; rep("reject",(r&&(r.stack||r.message))||String(r));
        });
        // 发送链路埋点：点击「发送」是否被页面收到、fetch 是否真的发出
        document.addEventListener("click",function(e){
          try{
            var t=e.target,k=0;
            while(t&&k<4){
              var lab=(t.getAttribute&&t.getAttribute("aria-label")||"")+"|"+
                      (t.children&&t.children.length===0?(t.textContent||""):"");
              if(lab.indexOf("发送")>=0){ rep("tap","send:"+lab.slice(0,40)); break; }
              t=t.parentElement; k++;
            }
          }catch(x){}
        },true);
        if(window.fetch){
          var _f0=window.fetch;
          window.fetch=function(a,b){
            try{
              var u=(a&&typeof a==="object"&&a.url)?a.url:String(a);
              if(u&&u.indexOf("prompt")>=0) rep("fetch","prompt:"+u.slice(0,200));
              return _f0.apply(this,arguments);
            }catch(e){ rep("fetcherr",String(e&&e.message||e)); throw e; }
          };
        }
      }catch(e){}
      setTimeout(function(){
        try{
          var b=document.body||document.documentElement, cs=getComputedStyle(b);
          var txt=((document.body&&document.body.innerText)||"").length;
          var all=document.querySelectorAll("*"), vis=0, i;
          for(i=0;i<all.length&&i<4000;i++){
            var r=all[i].getBoundingClientRect();
            if(r.width>2&&r.height>2)vis++;
          }
          var base="";
          try{ var bs=document.getElementsByTagName("base"); if(bs&&bs[0])base=bs[0].href||""; }catch(e){}
          rep("snap",JSON.stringify({
            ua:navigator.userAgent,w:innerWidth,h:innerHeight,dpr:devicePixelRatio,
            base:base,path:location.pathname,textLen:txt,vis:vis,all:all.length,
            bg:cs.backgroundColor,color:cs.color,
            ldm:(window.CSS&&CSS.supports)?CSS.supports("color","light-dark(white,black)"):"n/a"
          }));
        }catch(e){ rep("snaperr",String(e&&e.message||e)); }
      },7000);
      setTimeout(function(){
        try{
          var b=document.body||document.documentElement;
          var txt=((b&&b.innerText)||"").replace(/\s+/g," ").slice(0,400);
          var ce=document.querySelectorAll("[contenteditable=true]").length;
          var spin=document.querySelectorAll("svg animate,[class*=spinner],[class*=loading],[class*=skeleton]").length;
          rep("snap2",JSON.stringify({
            path:location.pathname,w:innerWidth,h:innerHeight,dpr:devicePixelRatio,
            txt:txt,ce:ce,spin:spin,els:document.querySelectorAll("*").length,
            ready:document.readyState,
            send:(function(){var b=[...document.querySelectorAll('button')]
              .find(function(x){return (x.textContent||'').trim()==='发送';});
              return b?(b.disabled?'disabled':'enabled'):'no-btn';})(),
            ctrls:[...document.querySelectorAll('button')]
              .map(function(x){return x.getAttribute('aria-label')||'';})
              .filter(function(v){return /模型|智能体|变体|图片/.test(v);}).slice(0,6),
            api:JSON.stringify(window.__gwApi||{}).slice(0,260)
          }));
        }catch(e){ rep("snap2err",String(e&&e.message||e)); }
      },25000);
    })();
    var P0=location.pathname;
    // 门户环境判定：地址栏带前缀，或服务端注入的 <base href> 指向门户前缀。
    // 手机 APP 经外网中转时地址栏可能被改写成 "/"，只看 location.pathname 会误判为
    // 直连入口，整段补丁不生效，前端按 location.origin 拼出的 /api/* 就到不了应用。
    var _bh="";
    try{
      var _bs=document.getElementsByTagName("base");
      if(_bs&&_bs[0]&&_bs[0].getAttribute)_bh=_bs[0].getAttribute("href")||"";
    }catch(e){}
    if(P0.indexOf(PB)!==0 && _bh.indexOf(PB)<0)return;
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
    // 事件流合并：HTTP/1.1 下浏览器对同一来源只给 6 条连接，而前端打开会话后
    // 会同时建 5~6 条 /api/event，把连接池占满 —— 之后发消息的 POST 只能排队，
    // 表现就是「点了发送没反应、服务端日志里连请求都没有」。
    // 这里把同一 URL 的重复订阅合并到一条真实连接，按订阅者分流**原始字节**：
    // 不新增事件、不改事件内容、不重放丢失事件，只把一条流分给多个消费者；
    // 任何异常都退回普通 fetch，不影响功能。
    var _hubs={};
    function isEventUrl(u){ return typeof u==="string" && /\/api\/event(\?|$)/.test(u); }
    function startHub(url){
      var hub={url:url,subs:[],dead:false,head:"",enc:null};
      var ctrl=null;
      try{ ctrl=new AbortController(); }catch(e){}
      function flush(){
        for(var i=0;i<hub.subs.length;i++){ try{ hub.subs[i].ctl.close(); }catch(e){} }
        hub.subs.length=0; hub.dead=true;
        if(_hubs[url]===hub)delete _hubs[url];
      }
      try{
        _f.call(window,url,ctrl?{signal:ctrl.signal}:undefined).then(function(res){
          if(!res||!res.body||!res.body.getReader){ flush(); return; }
          var reader=res.body.getReader(), dec=null;
          try{ dec=new TextDecoder(); hub.enc=new TextEncoder(); }catch(e){}
          (function pump(){
            reader.read().then(function(r){
              if(r.done){ flush(); return; }
              var chunk=r.value;
              if(dec&&hub.head.length<4096){
                try{
                  hub.head+=dec.decode(chunk,{stream:true});
                  var cut=hub.head.lastIndexOf("\n\n");
                  if(cut>0)hub.head=hub.head.slice(0,cut+2);
                  if(hub.head.length>4096)hub.head=hub.head.slice(0,4096);
                }catch(e){}
              }
              for(var i=0;i<hub.subs.length;i++){
                try{ hub.subs[i].ctl.enqueue(new Uint8Array(chunk)); }catch(e){}
              }
              pump();
            }).catch(function(){ flush(); });
          })();
        }).catch(function(){ flush(); });
      }catch(e){ flush(); }
      return hub;
    }
    function sseSubscribe(url){
      try{
        if(typeof ReadableStream==="undefined"||typeof Response==="undefined")throw 0;
        var hub=_hubs[url];
        if(!hub||hub.dead){ hub=startHub(url); _hubs[url]=hub; }
        var sub={ctl:null};
        var stream=new ReadableStream({
          start:function(c){
            sub.ctl=c; hub.subs.push(sub);
            // 后加入的订阅者补发已收到的开头（只补原样的握手帧，切在完整帧边界）
            if(hub.head&&hub.enc){ try{ c.enqueue(hub.enc.encode(hub.head)); }catch(e){} }
          },
          cancel:function(){ var i=hub.subs.indexOf(sub); if(i>=0)hub.subs.splice(i,1); }
        });
        return Promise.resolve(new Response(stream,{
          status:200,
          headers:{"Content-Type":"text/event-stream","Cache-Control":"no-cache","X-Accel-Buffering":"no"}
        }));
      }catch(e){ return null; }
    }
    // 前端 SDK 用 new Request(e,t) 构造请求，随后还会 new Request(旧Request,{signal})。
    // 在构造阶段就把字符串 URL 补成门户前缀，后续重建出来的 URL 天然带前缀，
    // fetch 就不必再重建 Request——手机 WebView 上最容易在这一步丢 body 或直接失败。
    try{
      var _R=window.Request;
      if(_R){
        var NR=function(input,init){
          try{ if(typeof input==="string")input=fix(input); }catch(e){}
          return new _R(input,init);
        };
        NR.prototype=_R.prototype;
        window.Request=NR;
      }
    }catch(e){}
    // 失效会话自愈：手机端保存了上次打开的会话地址，会话被删后应用拿到 404，
    // 界面只显示标题和旧消息、输入框直接不渲染（表现为「历史上一直修不好的手机端」）。
    // 这里在会话详情请求返回 404 时清掉本地「上次路由」并回到应用首页；
    // 用 sessionStorage 做一次性保护，避免任何情况下形成重定向循环。
    function staleSession(){
      try{
        if(sessionStorage.getItem("__gw_stale_route")==="1")return;
        sessionStorage.setItem("__gw_stale_route","1");
      }catch(e){}
      try{
        for(var i=localStorage.length-1;i>=0;i--){
          var k=localStorage.key(i);
          if(k&&/last-route|last_route/i.test(k))localStorage.removeItem(k);
        }
      }catch(e){}
      setTimeout(function(){
        try{
          if(location.pathname.indexOf("/session/")>=0||location.href.indexOf("/session/")>=0)
            location.href=PB+"/";
        }catch(e){}
      },700);
    }
    function maybeStale(u,p){
      try{
        if(p&&p.then&&u&&/\/api\/session\/[^/?]+$/.test(u)){
          return p.then(function(res){
            try{ if(res&&res.status===404)staleSession(); }catch(e){}
            return res;
          });
        }
      }catch(e){}
      return p;
    }
    window.__gwApi={};
    var _f=window.fetch;
    if(_f){
      window.fetch=function(input,init){
        try{
          var _eu=(typeof input==="string")?fix(input):((input&&input.url)?fix(input.url):"");
          try{
            var _m=(_eu||"").match(/\/api\/([A-Za-z0-9_.-]+)/);
            if(_m){var _k=_m[1];window.__gwApi[_k]=(window.__gwApi[_k]||0)+1;}
          }catch(e){}
          if(_eu&&isEventUrl(_eu)){ var _ss=sseSubscribe(_eu); if(_ss)return _ss; }
          if(typeof input==="string"){return maybeStale(_eu,_f.call(this,fix(input),init));}
          if(input&&input.url){
            var nu=fix(input.url);
            if(nu===input.url)return _f.call(this,input,init);
            // Firefox 里 new Request(url, 旧Request) 会丢掉 POST body（cl=0），
            // Chromium 不会。这里把 body 读成文本，再用普通 fetch 发出：
            // URL 前缀照样补，请求体由浏览器重新序列化，避开浏览器差异。
            var self=this, req=input, p=null;
            try{ p=req.clone().text(); }catch(e){}
            if(!p){
              // 旧 WebView 可能没有 Request.clone()：退回重建，至少 URL 带前缀能到达应用，
              // 不会被门户根路径吞掉（此时若丢 body，网关侧还有空 body 兜底）。
              try{ return maybeStale(nu,_f.call(self,new window.Request(nu,req),init)); }catch(e2){ return maybeStale(nu,_f.call(self,req,init)); }
            }
            return p.then(function(txt){
              var opt={
                method:req.method, headers:req.headers,
                mode:req.mode, credentials:req.credentials,
                cache:req.cache, redirect:req.redirect,
                referrer:req.referrer, integrity:req.integrity,
                keepalive:req.keepalive, signal:req.signal
              };
              if(req.method!=="GET"&&req.method!=="HEAD")opt.body=txt;
              return maybeStale(nu,_f.call(self,nu,opt));
            });
          }
        }catch(e){}
        return maybeStale(_eu,_f.call(this,input,init));
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
    // Service Worker：上游自带 Workbox SW 注册在站点根作用域，会拦截请求并缓存页面，
    // 手机 APP 下表现为页面反复重载、事件流被拦断、一直转圈。网关已把 /sw.js 换成
    // 不拦截、不缓存的空 SW；这里再把历史注册全部注销，立即解除旧 SW 对该页面的控制。
    // 不在这里 reload：注销后请求立即改走网络，reload 只会让页面反复重载。
    try{
      if(navigator.serviceWorker&&navigator.serviceWorker.getRegistrations){
        navigator.serviceWorker.getRegistrations().then(function(rs){
          if(!rs||!rs.length)return;
          for(var i=0;i<rs.length;i++){try{rs[i].unregister();}catch(e){}}
        }).catch(function(){});
      }
    }catch(e){}

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
        // 手机 / 内嵌 WebView 下这段自动点击会误点「新建会话」，把界面卡在
        // /new-session「正在新建会话…」，输入框没有模型、发送永远禁用。
        // 桌面保留自动打开项目，手机端交给用户手动点，行为与官方一致。
        var _ua=(navigator.userAgent||"").toLowerCase();
        if(/harmonyos|openharmony|arkweb|android|iphone|ipad/.test(_ua))return;
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
    // 清理浏览器本地残留：失效的 session id、指向不可读目录的项目路径。
    // 这些只存在 localStorage / sessionStorage 里，服务端清不到；不清会每次打开
    // 都带着旧状态请求，反复出现「找不到此会话」「Expected object」。
    try{
      setTimeout(function(){
        var stores=[];
        try{ if(window.localStorage)stores.push(window.localStorage); }catch(e){}
        try{ if(window.sessionStorage)stores.push(window.sessionStorage); }catch(e){}
        var sid=(P0.match(/ses_[A-Za-z0-9]+/)||[])[0];
        if(sid){
          fetch(fix("/api/session/"+sid)).then(function(r){
            if(r.status!==404)return;
            for(var s=0;s<stores.length;s++){
              try{
                for(var i=stores[s].length-1;i>=0;i--){
                  var k=stores[s].key(i),v=stores[s].getItem(k)||"";
                  if(k.indexOf(sid)>=0||v.indexOf(sid)>=0)stores[s].removeItem(k);
                }
              }catch(e){}
            }
          }).catch(function(){});
        }
        fetch(fix("/gateway/status")).then(function(r){return r.json();}).then(function(st){
          var wd=st&&st.workdir;
          if(!wd)return;
          for(var s=0;s<stores.length;s++){
            try{
              for(var i=0;i<stores[s].length;i++){
                var k=stores[s].key(i),v=stores[s].getItem(k)||"";
                if(v.indexOf("/vol1/1000")>=0)stores[s].setItem(k,v.split("/vol1/1000").join(wd));
              }
            }catch(e){}
          }
        }).catch(function(){});
      },900);
    }catch(e){}
    // 停留在已失效的会话页（后端已无该 session）时，点发送只会得到 400。
    // 检测到提示后自动点「关闭标签页」回到列表，避免用户卡在这里。
    try{
      setTimeout(function(){
        var t=(document.body&&document.body.innerText)||"";
        if(t.indexOf("找不到此会话")<0&&t.indexOf("Session not found")<0&&t.indexOf("会话已不存在")<0)return;
        var bs=document.getElementsByTagName("button");
        for(var i=0;i<bs.length;i++){
          var tx=(bs[i].innerText||"")+" "+(bs[i].getAttribute("aria-label")||"");
          if(tx.indexOf("关闭标签页")>=0||/close\s*tab/i.test(tx)){bs[i].click();return;}
        }
      },1500);
    }catch(e){}
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
