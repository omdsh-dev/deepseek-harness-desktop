// bridge.js — 由壳（gateway）经代理注入到 dsh 前端 index.html 的 <head>
// （runtime.js 之后），先于任何前端 bundle 执行。完全接管
// window.localStorage：读写全部走壳内共享存储（wails 绑定
// LocalStorage.Set/Remove/Clear，经网关 /wails/runtime IPC），不碰原生
// webview localStorage——「localStorage 是共享层的，不是 webview 的」。
//
// 数据流：
//   - 初始状态：网关注入时把共享存储当前快照内嵌为种子（有序 [key,value]
//     数组，如 dsh.sessions.current），页面启动 getItem 同步读到，不依赖
//     异步 wails IPC。种子只作为初始状态。
//   - 后续写：setItem/removeItem/clear 同步更新内存缓存后经 wails 绑定
//     落盘共享层；wails 未就绪（runtime.js 是延迟 module，解析完成后才
//     设置 window.wails）时写入暂存队列，就绪后按序回放——写永远不落原生。
//   - 后续读：读内存缓存（种子 + 本页写入；桌面单实例下与共享层一致）。
//   - 无降级：wails 不可用（runtime.js 加载失败、网关退化）时 localStorage
//     也随之不可用——写暂存不落盘、读只有缓存，绝不静默回退 webview 原生
//     存储（那会让共享层与原生层分叉，违背接管语义）。
//
// 绑定 FQN：github.com/omdsh-dev/dsh-web-desktopify/pkg/shell/sharedstore.Store.<Method>
(function () {
	if (window.__dshSharedLocalStorageInstalled) return;
	window.__dshSharedLocalStorageInstalled = true;
	if (typeof window.localStorage === "undefined") return;

	// 种子：网关在注入时把共享存储当前状态内嵌为有序 [key, value] 数组，
	// 只作为初始状态。
	var __seed = /*__DSH_SHARED_SEED__*/;
	var cache = {};   // 共享层内存镜像：key -> value
	var keys = [];    // key 顺序（key()/length 用）
	for (var i = 0; i < __seed.length; i++) {
		var kv = __seed[i];
		cache[kv[0]] = kv[1];
		keys.push(kv[0]);
	}

	function haveWails() {
		return !!(window.wails && window.wails.Call && window.wails.Call.ByName);
	}

	// wails 未就绪时的写暂存队列：就绪后按序回放落盘。wails 一直不来则
	// 一直暂存（不落原生、不丢到 webview 存储）。
	var pending = [];
	var tick = null;

	// 经 wails 绑定写共享层。失败静默（本地回环，极罕见）。
	function fire(method, args) {
		try {
			window.wails.Call.ByName.apply(window.wails.Call, [
				"github.com/omdsh-dev/dsh-web-desktopify/pkg/shell/sharedstore.Store." + method
			].concat(args));
		} catch (e) {}
	}

	function flushPending() {
		if (pending.length === 0) return;
		var queued = pending;
		pending = [];
		for (var i = 0; i < queued.length; i++) fire(queued[i][0], queued[i][1]);
	}

	// 暂存非空时轮询 wails 就绪并回放；队列清空即停。
	function ensureTick() {
		if (tick !== null) return;
		tick = setInterval(function () {
			if (pending.length === 0) {
				clearInterval(tick);
				tick = null;
				return;
			}
			if (haveWails()) flushPending();
		}, 200);
	}

	function call(method, args) {
		if (haveWails()) {
			fire(method, args);
			flushPending();
		} else {
			pending.push([method, args]);
			ensureTick();
		}
	}

	function indexOf(k) {
		for (var i = 0; i < keys.length; i++) if (keys[i] === k) return i;
		return -1;
	}

	var api = {
		getItem: function (k) {
			if (k in cache) return cache[k];
			return null; // 历史空了就空了
		},
		setItem: function (k, v) {
			var s = String(v);
			if (!(k in cache)) keys.push(k);
			cache[k] = s;
			call("Set", [k, s]);
		},
		removeItem: function (k) {
			if (k in cache) {
				delete cache[k];
				var at = indexOf(k);
				if (at !== -1) keys.splice(at, 1);
			}
			call("Remove", [k]);
		},
		clear: function () {
			cache = {};
			keys = [];
			call("Clear", []);
		},
		key: function (i) {
			return i >= 0 && i < keys.length ? keys[i] : null;
		},
		get length() {
			return keys.length;
		}
	};
	Object.defineProperty(window, "localStorage", {
		value: api, configurable: true, enumerable: true, writable: true
	});
})();
