// Seeds a running server with demo users 张三/李四/王五, a sample project and
// two paired devices. Prints shell assignments for the node tokens.
//   node scripts/seed-preview.mjs URL REMOTE PASSWORD
const [url, remote, password] = process.argv.slice(2);

async function call(method, path, body, s) {
  const headers = { "Content-Type": "application/json" };
  if (s) Object.assign(headers, { Authorization: `Bearer ${s.token}`, "X-Workspace-ID": s.workspace_id });
  const r = await fetch(url + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  const j = await r.json();
  if (!r.ok) throw new Error(`${method} ${path}: ${r.status} ${JSON.stringify(j)}`);
  return j;
}
const login = (email, name, handle, invite_code = "") => call("POST", "/api/auth/dev-login", { email, name, handle, invite_code, password });

const zs = await login("zhangsan@demo.local", "张三", "zhangsan");
await call("POST", "/api/projects", { name: "calc", remote_url: remote, default_branch: "main" }, zs);
const me = await call("GET", "/api/me", undefined, zs);
const invite = me.workspaces[0].invite_code;
const ls = await login("lisi@demo.local", "李四", "lisi", invite);
await login("wangwu@demo.local", "王五", "wangwu", invite);
const a = await call("POST", "/api/daemons", { name: "zhangsan-laptop", agents: [{ handle: "zs-chief", kind: "chief" }, { handle: "zs-coder", kind: "specialist" }] }, zs);
const b = await call("POST", "/api/daemons", { name: "lisi-laptop", agents: [{ handle: "ls-coder", kind: "specialist" }] }, ls);
console.log(`ZS_TOKEN=${a.node_token}`);
console.log(`LS_TOKEN=${b.node_token}`);
