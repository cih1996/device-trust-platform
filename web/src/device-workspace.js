import template from './device-workspace.html?raw'
import ResourcePanel from './resource-panel.js'
const packageNames={android:'系统框架','com.android.settings':'系统设置','com.android.systemui':'系统界面','com.miui.securitycenter':'小米安全中心','com.miui.home':'小米桌面','com.xiaomi.market':'小米应用商店','com.huawei.hwid':'华为移动服务','com.huawei.appmarket':'华为应用市场','com.heytap.market':'欢太软件商店','com.oplus.safecenter':'系统安全中心','com.bbk.appstore':'vivo 应用商店','com.bbk.launcher2':'vivo 桌面','com.hihonor.appmarket':'荣耀应用市场'}
export function observed(report,key){const signal=report?.signals?.[key];return signal?.status==='available'?signal.value:null}
export function gib(value){return typeof value==='number'&&value>=0?(value/1073741824).toFixed(2)+' GiB':'未提供'}
export default{
 props:['api','appId','apps'],components:{ResourcePanel},template,
 data:()=>({listing:{items:[],total:0,stats:{total:0,active:0,conflict:0,incomplete:0}},query:'',offset:0,selected:null,tab:'overview',busy:false,error:''}),
 mounted(){this.load()},watch:{appId(){this.offset=0;this.selected=null;this.load()}},
 computed:{
  p(){return this.selected?.profile},r(){return this.p?.latestReport},
  java(){return observed(this.r,'resourceJava')},native(){return observed(this.r,'resourceNative')},
  packages(){return observed(this.r,'systemPackages')||[]},sensors(){return observed(this.r,'sensors')||[]},
  groups(){const j=this.java,n=this.native,os=observed(this.r,'osInfo'),mem=observed(this.r,'memory'),storage=observed(this.r,'storage'),screen=observed(this.r,'display'),battery=observed(this.r,'battery');return [
   {name:'机型与处理器',fields:[['品牌',j?.brand],['制造商',j?.manufacturer],['型号',j?.model],['设备代号',j?.device],['产品',j?.product],['主板',j?.board],['CPU 型号',n?.cpuHardware],['硬件平台',n?.platform],['系统架构',n?.machine],['处理器架构',j?.supportedAbis?.join(' / ')],['配置核心',n?.configuredCores],['应用可用核心',j?.availableCores],['SoC 厂商',j?.socManufacturer],['SoC 型号',j?.socModel]]},
   {name:'内存与存储',fields:[['运行内存',mem?gib(mem.totalBytes):gib(j?.memoryBytes)],['当前可用内存',gib(mem?.availableBytes)],['数据分区容量',gib(storage?.totalBytes)],['分区可用空间',gib(storage?.availableBytes)],['内存紧张',mem?.lowMemory]]},
   {name:'系统与显示',fields:[['Android 版本',os?.release],['API 版本',os?.sdk],['安全更新',os?.patch],['系统版本',os?.display],['构建类型',os?.type],['内核',n?.kernelRelease],['启动程序',os?.bootloader],['开机时长',os?.uptimeMillis!=null?Math.floor(os.uptimeMillis/3600000)+' 小时':null],['显示区域',screen?screen.widthPixels+' × '+screen.heightPixels:null],['显示密度',screen?.densityDpi!=null?screen.densityDpi+' dpi':null],['字体缩放',screen?.fontScale],['系统指纹',observed(this.r,'buildFingerprint')]]},
   {name:'电源状态',fields:[['当前电量',battery&&battery.scale>0&&battery.level>=0?Math.round(battery.level/battery.scale*100)+'%':null],['电池温度',battery?.temperatureTenthsC>=0?battery.temperatureTenthsC/10+' ℃':null],['电池电压',battery?.voltageMillivolts>0?battery.voltageMillivolts+' mV':null],['供电方式',battery?({0:'电池',1:'交流电',2:'USB',4:'无线充电'}[battery.plugged]||'其他'):null]]}
  ]},
  application(){return observed(this.r,'application')},
  network(){return observed(this.r,'networkSnapshot')},gps(){return observed(this.r,'location')},cells(){return observed(this.r,'cell')||[]},
  networkHistory(){const rows=[];for(const report of this.selected?.reports||[]){const last=rows[rows.length-1];if(!last||last.ip!==report.observedIp||last.transport!==observed(report,'transport'))rows.push({time:report.receivedAt,ip:report.observedIp,transport:observed(report,'transport'),region:report.resources?.ip})}return rows},
  completeness(){if(!this.r)return {available:0,total:0};const values=Object.values(this.r.signals);return {available:values.filter(s=>s.status==='available').length,total:values.length}}
 },
 methods:{
  async load(){this.busy=true;this.error='';try{this.listing=await this.api('devices?appId='+encodeURIComponent(this.appId||'')+'&q='+encodeURIComponent(this.query)+'&offset='+this.offset+'&limit=50')}catch(e){this.error=e.message}finally{this.busy=false}},
  search(){this.offset=0;this.load()},page(delta){this.offset+=delta*50;this.load()},
  reload(){if(this.selected)this.open(this.p);else this.load()},
  async open(profile){this.busy=true;this.error='';try{this.selected=await this.api('devices/'+profile.id);this.tab='overview'}catch(e){this.error=e.message}finally{this.busy=false}},
  close(){this.selected=null},
  model(profile){return observed(profile.latestReport,'model')||'待识别机型'},brand(profile){return observed(profile.latestReport,'manufacturer')||'品牌未提供'},
  hardware(profile){const value=observed(profile.latestReport,'resourceNative');return value?[value.platform,gib(value.memoryBytes),value.configuredCores?value.configuredCores+' 核':null].filter(Boolean).join(' · '):'硬件信息未上报'},
  appName(id){return this.apps.find(app=>app.id===id)?.name||id},
  date(value){return new Date(value).toLocaleString('zh-CN',{hour12:false})},
  display(value){if(value===null||value===undefined||value==='')return '未提供';if(typeof value==='boolean')return value?'是':'否';if(typeof value==='number'&&!Number.isInteger(value))return String(Number(value.toPrecision(6)));return String(value)},
  sensorType(type){return {1:'加速度',2:'磁场',3:'方向',4:'陀螺仪',5:'光线',8:'距离',9:'重力',10:'线性加速度',11:'旋转矢量',14:'未校准磁场',15:'游戏旋转',16:'未校准陀螺仪',17:'显著运动',18:'步行检测',19:'计步器',20:'地磁旋转',22:'倾斜检测',27:'设备方向',29:'静止检测',30:'运动检测',35:'未校准加速度'}[type]||'厂商扩展'},
  purpose(value){return {device_init:'首次登记',device_refresh:'启动更新',register:'注册场景',reward:'营销场景',payment:'支付场景'}[value]||'业务上报'},
  proof(value){return !value?'未提供证明':value.level==='tee'||value.level==='strongbox'?'硬件证明通过':'签名已验证'},
  transport(value){return {wifi:'Wi-Fi',cellular:'移动网络',ethernet:'有线网络',other:'其他网络'}[value]||'暂不可用'},
  state(profile){if(profile.keyStatus==='revoked')return '密钥已停用';const checks=profile.latestReport.resources?.checks;if(!checks)return '待比对';if(checks.some(c=>c.state==='conflict'))return '存在矛盾';if(checks.some(c=>['unknown','stale','error'].includes(c.state)))return observed(profile.latestReport,'resourceJava')?'部分已比对':'信息待补充';return '比对一致'},
  stateClass(profile){if(profile.keyStatus==='revoked'||profile.latestReport.resources?.checks.some(c=>c.state==='conflict'))return 'conflict';return profile.latestReport.resources?.checks.some(c=>['unknown','stale','error'].includes(c.state))?'incomplete':'matched'},
  signalStatus(key){const signal=this.r?.signals?.[key];return signal?({available:'已采集',disabled:'未开启',permission_denied:'未授权',unavailable:'暂不可用',error:'采集出错'}[signal.status]||signal.status):'未上报'},
  signalReason(key){return this.r?.signals?.[key]?.reason||''},
  packageName(value){return packageNames[value]||value},
  exportRecord(){const blob=new Blob([JSON.stringify(this.selected,null,2)],{type:'application/json'});const url=URL.createObjectURL(blob);const a=document.createElement('a');a.href=url;a.download='DeviceTrust-'+(this.p.deviceId||this.p.id)+'.json';a.click();URL.revokeObjectURL(url)},
  async revoke(){if(!window.confirm('停用此设备密钥？'))return;this.busy=true;this.error='';try{await this.api('device-keys/revoke','POST',{appId:this.p.appId,keyId:this.r.proof.keyId,reason:'operator'});this.close();await this.load()}catch(e){this.error=e.message}finally{this.busy=false}}
 }
}
