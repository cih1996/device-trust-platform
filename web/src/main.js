import { createApp, ref, computed, onMounted } from 'vue/dist/vue.esm-bundler.js'
import './style.css'
import template from './template.html?raw'
import ResourcePanel from './resource-panel.js'
import DeviceWorkspace from './device-workspace.js'
import { signalValue } from './signals.js'
import { sceneNames, actionNames, reviewNames, riskReasons, fieldNames, outcomeNames } from './risk-display.js'

createApp({
 components:{ResourcePanel,DeviceWorkspace},
 setup() {
  const savedSession=window.localStorage.getItem('dtp_admin_session')||window.sessionStorage.getItem('dtp_admin_session')
  const token=ref(savedSession||''), signedIn=ref(false), apps=ref([]), overview=ref({stats:{events:0,devices:0,review:0},analytics:{averageRisk:0,riskBuckets:{},activity:[],topIPs:[]},events:[],accesses:[]})
  const username=ref('admin'), password=ref(''), currentPassword=ref(''), newPassword=ref(''), confirmPassword=ref(''), settingsOpen=ref(false), managedApp=ref(null)
  const selected=ref(''), section=ref('apps'), error=ref(''), busy=ref(false), name=ref(''), packageName=ref(''), newKey=ref(''), query=ref(''), detail=ref(null)
  const sectionNames={overview:'风险概览',events:'风险事件',devices:'设备档案',apps:'应用选择'}
  const signalNames={resourceJava:'设备参数',resourceNative:'底层参数',sensors:'传感器',systemPackages:'系统应用',networkSnapshot:'网络状态',manufacturer:'制造商',model:'机型',sdkInt:'系统版本',securityPatch:'安全更新',buildFingerprint:'系统版本标识',debugger:'调试状态',emulatorHint:'模拟环境线索',vpn:'VPN 连接',transport:'网络连接',operator:'网络运营商',androidId:'应用设备标识',expectedPackage:'应用匹配',cell:'基站与信号',wifiRssi:'Wi-Fi 信号',bssid:'无线接入点',location:'位置',installedApps:'已安装应用',bluetoothMac:'蓝牙地址',routerMac:'路由地址',attestation:'硬件证明',rootBeerTestKeys:'测试版系统线索',rootBeerSuBinary:'提权程序线索',rootBeerMagiskBinary:'系统修改工具线索',rootBeerNativeSu:'原生提权程序线索',rootBeerVersion:'采集组件版本'}
  const reasons=riskReasons
  const deviceWorkspace=ref(null)
  const scenario=ref(''),policyApp=ref(null),policyDraft=ref(null),reviewNote=ref('')
  const securityApp=ref(null),securityDraft=ref(null),signerText=ref(''),resolutionId=ref('')
  const workspaceMode=computed(()=>{const list=apps.value.filter(app=>!selected.value||app.id===selected.value);return list.some(app=>app.policy.mode==='enforce')?'执行模式':'观察模式'})
  const proofName=proof=>!proof?'待验证':(proof.level==='tee'||proof.level==='strongbox')?'硬件密钥已验证':'签名已验证'
  function gateState(event){const execution=overview.value.executions[event.id];if(execution)return execution.status==='succeeded'?'业务已完成':'执行失败';const resolution=overview.value.resolutions[event.id];if(resolution)return resolution.status==='approved'?'已批准':'已拒绝';if(event.mode!=='enforce')return '观察';return event.action==='hold'?'待审核':event.action==='verify'?'待核验':'可执行'}
  const policyFields=[['reviewScore','建议核验分数'],['holdScore','建议暂缓分数'],['registrationAccounts','设备注册账号数 · 10 分钟'],['ipRegistrations','网络注册账号数 · 10 分钟'],['campaignAccounts','活动关联账号数 · 24 小时'],['paymentMethods','账号支付工具数 · 24 小时'],['paymentAccounts','支付工具关联账号数 · 24 小时'],['paymentFailures','支付失败次数 · 1 小时']]
  const events=computed(()=>overview.value.events.filter(e=>(!scenario.value||e.scene===scenario.value)&&[e.accountId,e.type,e.installationId].some(v=>v.includes(query.value))))
  async function api(path,method='GET',body) {
   const response=await fetch('/api/'+path,{method,headers:{Authorization:'Bearer '+token.value,'Content-Type':'application/json'},body:body?JSON.stringify(body):undefined})
   const data=await response.json();if(!response.ok) throw Error(data.error);return data
  }
  async function act(action){error.value='';busy.value=true;try{await action()}catch(e){error.value=e.message}finally{busy.value=false}}
  function routeHash(path){const next='#/'+path.replace(/^\//,'');if(location.hash!==next)history.pushState({},'',next)}
  function readRoute(){const parts=location.hash.replace(/^#\/?/,'').split('/').filter(Boolean);if(parts[0]==='app'&&parts[1]){selected.value=decodeURIComponent(parts[1]);const page=parts[2]||'overview';section.value=page==='settings'?'apps':page;managedApp.value=null}else{selected.value='';section.value='apps';managedApp.value=null;if(!location.hash)history.replaceState({},'', '#/apps')}}
  function navigate(page){if(!selected.value){routeHash('apps');section.value='apps';return}managedApp.value=null;section.value=page;routeHash('app/'+encodeURIComponent(selected.value)+'/'+page)}
  async function refresh(){apps.value=await api('apps');if(section.value==='apps'&&selected.value)managedApp.value=apps.value.find(app=>app.id===selected.value)||null;overview.value=await api('overview?appId='+encodeURIComponent(selected.value))}
  function login(){act(async()=>{const response=await fetch('/api/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:username.value,password:password.value})});const data=await response.json();if(!response.ok)throw Error(data.error);token.value=data.token;window.localStorage.setItem('dtp_admin_session',data.token);window.sessionStorage.setItem('dtp_admin_session',data.token);password.value='';await refresh();signedIn.value=true})}
  async function restoreSession(){readRoute();if(!token.value)return;try{await refresh();signedIn.value=true}catch(err){token.value='';window.localStorage.removeItem('dtp_admin_session');window.sessionStorage.removeItem('dtp_admin_session');error.value=''}}
  function changeApp(){act(refresh)}
  function clearApp(){act(async()=>{selected.value='';managedApp.value=null;section.value='apps';routeHash('apps');await refresh()})}
  function enterApp(app){act(async()=>{managedApp.value=null;selected.value=app.id;section.value='overview';routeHash('app/'+encodeURIComponent(app.id)+'/overview');await refresh()})}
  function openAppSettings(app){act(async()=>{managedApp.value=app;selected.value=app.id;section.value='apps';routeHash('app/'+encodeURIComponent(app.id)+'/settings');await refresh()})}
  function exitApp(){managedApp.value=null;selected.value='';section.value='apps';routeHash('apps');act(refresh)}
  function goAppOverview(){managedApp.value=null;navigate('overview')}
  function reload(){if(section.value==='devices')deviceWorkspace.value?.reload();else act(refresh)}
  function openPolicy(app){policyApp.value=app;policyDraft.value={...app.policy}}
  function openSecurity(app){securityApp.value=app;securityDraft.value={...app.security};signerText.value=app.security.signingDigests.join('\n')}
  function saveSecurity(){act(async()=>{await api('apps/'+securityApp.value.id+'/security','PUT',{...securityDraft.value,signingDigests:signerText.value.split(/\s+/).filter(Boolean)});securityApp.value=null;await refresh()})}
  function resolve(status){act(async()=>{await api('events/'+detail.value.id+'/resolve','POST',{resolutionId:resolutionId.value,status,note:reviewNote.value});await refresh()})}
  function savePolicy(){act(async()=>{await api('apps/'+policyApp.value.id+'/policy','PUT',policyDraft.value);policyApp.value=null;await refresh()})}
  function openEvent(event){detail.value=event;reviewNote.value=overview.value.feedback[event.id]?.note||'';resolutionId.value=crypto.randomUUID()}
  function review(label){act(async()=>{await api('events/'+detail.value.id+'/reviews','POST',{label,note:reviewNote.value});await refresh()})}
  function create(){act(async()=>{const data=await api('apps','POST',{name:name.value,package:packageName.value});newKey.value=data.key;name.value='';packageName.value='';await refresh()})}
  function rotate(app){if(window.confirm('重置「'+app.name+'」的密钥？原密钥将立即失效。'))act(async()=>{const data=await api('apps/'+app.id+'/rotate-key','POST',{});newKey.value=data.key})}
  function changePassword(){act(async()=>{if(newPassword.value!==confirmPassword.value)throw Error('两次输入的新密码不一致');await api('auth/password','PUT',{currentPassword:currentPassword.value,newPassword:newPassword.value});currentPassword.value='';newPassword.value='';confirmPassword.value='';settingsOpen.value=false})}
  function logout(){token.value='';window.localStorage.removeItem('dtp_admin_session');window.sessionStorage.removeItem('dtp_admin_session');signedIn.value=false;section.value='apps';selected.value='';newKey.value='';detail.value=null;managedApp.value=null;apps.value=[];overview.value={stats:{events:0,devices:0,review:0},analytics:{averageRisk:0,riskBuckets:{},activity:[],topIPs:[]},events:[],accesses:[]}}
  const accesses=computed(()=> (overview.value.accesses||[]).filter(e=>(!query.value||[e.accountId,e.type,e.deviceId,e.ip].some(v=>String(v||'').includes(query.value)))))
  const scoreWidth=score=>Math.max(0,Math.min(100,Number(score)||0))
  const riskClass=score=>score>=70?'high':score>=30?'medium':'low'
  const riskLabel=score=>score>=70?'高风险':score>=30?'中风险':'低风险'
  const riskTooltip=access=>`风险综合评分 ${access.score}/100 · ${riskLabel(access.score)} · ${access.ip||'未提供 IP'} · ${access.deviceId||'未登记设备'}`
  function openAccess(access){const event=overview.value.events.find(item=>item.id===access.id);if(event)openEvent(event)}
  onMounted(()=>{window.addEventListener('popstate',readRoute);window.addEventListener('hashchange',readRoute);restoreSession()})
  const appName=id=>apps.value.find(a=>a.id===id)?.name||id
  const formatTime=value=>new Date(value).toLocaleString('zh-CN',{hour12:false})
  return {api,deviceWorkspace,changeApp,clearApp,enterApp,openAppSettings,exitApp,goAppOverview,navigate,managedApp,token,username,password,currentPassword,newPassword,confirmPassword,settingsOpen,signedIn,apps,overview,selected,section,error,busy,name,packageName,newKey,query,detail,sectionNames,signalNames,signalValue,reasons,events,accesses,login,reload,create,rotate,logout,changePassword,appName,formatTime,sceneNames,actionNames,reviewNames,fieldNames,outcomeNames,scenario,policyApp,policyDraft,policyFields,reviewNote,openPolicy,savePolicy,openEvent,openAccess,review,securityApp,securityDraft,signerText,workspaceMode,proofName,gateState,openSecurity,saveSecurity,resolve,scoreWidth,riskClass,riskLabel,riskTooltip}
 },
 template
}).mount('#app')
