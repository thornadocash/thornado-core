import qrcode from "/thornado/ui/vendor/qrcode.min.js?v=esm";
import { nodeOrigin, requestJson } from "/thornado/ui/modules/api.js";
import { parseListInput } from "/thornado/ui/modules/forms.js";

let thornadoWasmReady;

async function thornadoWasm() {
  if (!thornadoWasmReady) {
    const wasmVersion = new URLSearchParams(window.location.search).get("v") || "local";
    const wasmUrl = new URL(`/thornado/ui/wasm/thornado_web_wasm.js?v=${encodeURIComponent(wasmVersion)}`, window.location.origin).href;
    const wasmBinaryUrl = new URL(`/thornado/ui/wasm/thornado_web_wasm_bg.wasm?v=${encodeURIComponent(wasmVersion)}`, window.location.origin).href;
    thornadoWasmReady = import(wasmUrl).then(async (mod) => {
      await mod.default(wasmBinaryUrl);
      return mod;
    });
  }
  const mod = await thornadoWasmReady;
  return {
    clientPubkeyFromSecretJson: mod.client_pubkey_from_secret_json,
    clientPubkeyForDepositJson: mod.client_pubkey_for_deposit_json,
    clientPubkeyForDepositTypeJson: mod.client_pubkey_for_deposit_type_json,
    shieldAuthorizationJson: mod.shield_authorization_json,
    shieldAuthorizationForDepositJson: mod.shield_authorization_for_deposit_json,
    shieldAuthorizationForDepositTypeJson: mod.shield_authorization_for_deposit_type_json,
    deriveShieldReceiptJson: mod.derive_shield_receipt_json,
    deriveShieldReceiptForDepositJson: mod.derive_shield_receipt_for_deposit_json,
    deriveShieldReceiptForDepositTypeJson: mod.derive_shield_receipt_for_deposit_type_json,
    noteRecoveryCandidatesJson: mod.note_recovery_candidates_json,
    recoverNoteReceiptJson: mod.recover_note_receipt_json,
    recoverNoteReceiptForDepositTypeJson: mod.recover_note_receipt_for_deposit_type_json,
    nullifierHashJson: mod.nullifier_hash_json,
    verifyWithdrawalJson: mod.verify_withdrawal_json,
    withdrawalWitnessFromReceiptJson: mod.withdrawal_witness_from_receipt_json,
    zkWithdrawalFromReceiptJson: mod.zk_withdrawal_from_receipt_json || mod.shielder_withdrawal_from_receipt_json
  };
}
const DOMAIN = "thornado-mvp-v1";
const POW_DIFFICULTY_BITS = 17;
const MIN_POW_MS = 2000;
const DEFAULT_MIN_CONFS = 1;
const POW_VISUAL_REFERENCE_MS = 10000;
const MS_PER_YEAR = 365 * 24 * 60 * 60 * 1000;
const DEFAULT_BLOCKS_PER_YEAR = 365 * 24 * 60 * 60;
const DENOMINATIONS = [1000000000, 100000000, 10000000, 1000000];
const WITHDRAWAL_FEE_BASIS_POINTS = 100;
const SHIELDER_NOTE_MIN_SATS = Math.min(...DENOMINATIONS);
const DEMO_MATURITY_MS = 60000;
const MIN_LATER_DEPOSITS = 3;
const POOL_REFRESH_MS = 15000;
const SHIELDER_SYNC_PAGE_LIMIT = 250;
const DEFAULT_CHURN_CYCLE_MS = 20 * 60 * 1000;
const BIP39_WORDS = ["abandon","ability","able","about","above","absent","absorb","abstract","absurd","abuse","access","accident","account","accuse","achieve","acid","acoustic","acquire","across","act","action","actor","actress","actual","adapt","add","addict","address","adjust","admit","adult","advance","advice","aerobic","affair","afford","afraid","again","age","agent","agree","ahead","aim","air","airport","aisle","alarm","album","alcohol","alert","alien","all","alley","allow","almost","alone","alpha","already","also","alter","always","amateur","amazing","among","amount","amused","analyst","anchor","ancient","anger","angle","angry","animal","ankle","announce","annual","another","answer","antenna","antique","anxiety","any","apart","apology","appear","apple","approve","april","arch","arctic","area","arena","argue","arm","armed","armor","army","around","arrange","arrest","arrive","arrow","art","artefact","artist","artwork","ask","aspect","assault","asset","assist","assume","asthma","athlete","atom","attack","attend","attitude","attract","auction","audit","august","aunt","author","auto","autumn","average","avocado","avoid","awake","aware","away","awesome","awful","awkward","axis","baby","bachelor","bacon","badge","bag","balance","balcony","ball","bamboo","banana","banner","bar","barely","bargain","barrel","base","basic","basket","battle","beach","bean","beauty","because","become","beef","before","begin","behave","behind","believe","below","belt","bench","benefit","best","betray","better","between","beyond","bicycle","bid","bike","bind","biology","bird","birth","bitter","black","blade","blame","blanket","blast","bleak","bless","blind","blood","blossom","blouse","blue","blur","blush","board","boat","body","boil","bomb","bone","bonus","book","boost","border","boring","borrow","boss","bottom","bounce","box","boy","bracket","brain","brand","brass","brave","bread","breeze","brick","bridge","brief","bright","bring","brisk","broccoli","broken","bronze","broom","brother","brown","brush","bubble","buddy","budget","buffalo","build","bulb","bulk","bullet","bundle","bunker","burden","burger","burst","bus","business","busy","butter","buyer","buzz","cabbage","cabin","cable","cactus","cage","cake","call","calm","camera","camp","can","canal","cancel","candy","cannon","canoe","canvas","canyon","capable","capital","captain","car","carbon","card","cargo","carpet","carry","cart","case","cash","casino","castle","casual","cat","catalog","catch","category","cattle","caught","cause","caution","cave","ceiling","celery","cement","census","century","cereal","certain","chair","chalk","champion","change","chaos","chapter","charge","chase","chat","cheap","check","cheese","chef","cherry","chest","chicken","chief","child","chimney","choice","choose","chronic","chuckle","chunk","churn","cigar","cinnamon","circle","citizen","city","civil","claim","clap","clarify","claw","clay","clean","clerk","clever","click","client","cliff","climb","clinic","clip","clock","clog","close","cloth","cloud","clown","club","clump","cluster","clutch","coach","coast","coconut","code","coffee","coil","coin","collect","color","column","combine","come","comfort","comic","common","company","concert","conduct","confirm","congress","connect","consider","control","convince","cook","cool","copper","copy","coral","core","corn","correct","cost","cotton","couch","country","couple","course","cousin","cover","coyote","crack","cradle","craft","cram","crane","crash","crater","crawl","crazy","cream","credit","creek","crew","cricket","crime","crisp","critic","crop","cross","crouch","crowd","crucial","cruel","cruise","crumble","crunch","crush","cry","crystal","cube","culture","cup","cupboard","curious","current","curtain","curve","cushion","custom","cute","cycle","dad","damage","damp","dance","danger","daring","dash","daughter","dawn","day","deal","debate","debris","decade","december","decide","decline","decorate","decrease","deer","defense","define","defy","degree","delay","deliver","demand","demise","denial","dentist","deny","depart","depend","deposit","depth","deputy","derive","describe","desert","design","desk","despair","destroy","detail","detect","develop","device","devote","diagram","dial","diamond","diary","dice","diesel","diet","differ","digital","dignity","dilemma","dinner","dinosaur","direct","dirt","disagree","discover","disease","dish","dismiss","disorder","display","distance","divert","divide","divorce","dizzy","doctor","document","dog","doll","dolphin","domain","donate","donkey","donor","door","dose","double","dove","draft","dragon","drama","drastic","draw","dream","dress","drift","drill","drink","drip","drive","drop","drum","dry","duck","dumb","dune","during","dust","dutch","duty","dwarf","dynamic","eager","eagle","early","earn","earth","easily","east","easy","echo","ecology","economy","edge","edit","educate","effort","egg","eight","either","elbow","elder","electric","elegant","element","elephant","elevator","elite","else","embark","embody","embrace","emerge","emotion","employ","empower","empty","enable","enact","end","endless","endorse","enemy","energy","enforce","engage","engine","enhance","enjoy","enlist","enough","enrich","enroll","ensure","enter","entire","entry","envelope","episode","equal","equip","era","erase","erode","erosion","error","erupt","escape","essay","essence","estate","eternal","ethics","evidence","evil","evoke","evolve","exact","example","excess","exchange","excite","exclude","excuse","execute","exercise","exhaust","exhibit","exile","exist","exit","exotic","expand","expect","expire","explain","expose","express","extend","extra","eye","eyebrow","fabric","face","faculty","fade","faint","faith","fall","false","fame","family","famous","fan","fancy","fantasy","farm","fashion","fat","fatal","father","fatigue","fault","favorite","feature","february","federal","fee","feed","feel","female","fence","festival","fetch","fever","few","fiber","fiction","field","figure","file","film","filter","final","find","fine","finger","finish","fire","firm","first","fiscal","fish","fit","fitness","fix","flag","flame","flash","flat","flavor","flee","flight","flip","float","flock","floor","flower","fluid","flush","fly","foam","focus","fog","foil","fold","follow","food","foot","force","forest","forget","fork","fortune","forum","forward","fossil","foster","found","fox","fragile","frame","frequent","fresh","friend","fringe","frog","front","frost","frown","frozen","fruit","fuel","fun","funny","furnace","fury","future","gadget","gain","galaxy","gallery","game","gap","garage","garbage","garden","garlic","garment","gas","gasp","gate","gather","gauge","gaze","general","genius","genre","gentle","genuine","gesture","ghost","giant","gift","giggle","ginger","giraffe","girl","give","glad","glance","glare","glass","glide","glimpse","globe","gloom","glory","glove","glow","glue","goat","goddess","gold","good","goose","gorilla","gospel","gossip","govern","gown","grab","grace","grain","grant","grape","grass","gravity","great","green","grid","grief","grit","grocery","group","grow","grunt","guard","guess","guide","guilt","guitar","gun","gym","habit","hair","half","hammer","hamster","hand","happy","harbor","hard","harsh","harvest","hat","have","hawk","hazard","head","health","heart","heavy","hedgehog","height","hello","helmet","help","hen","hero","hidden","high","hill","hint","hip","hire","history","hobby","hockey","hold","hole","holiday","hollow","home","honey","hood","hope","horn","horror","horse","hospital","host","hotel","hour","hover","hub","huge","human","humble","humor","hundred","hungry","hunt","hurdle","hurry","hurt","husband","hybrid","ice","icon","idea","identify","idle","ignore","ill","illegal","illness","image","imitate","immense","immune","impact","impose","improve","impulse","inch","include","income","increase","index","indicate","indoor","industry","infant","inflict","inform","inhale","inherit","initial","inject","injury","inmate","inner","innocent","input","inquiry","insane","insect","inside","inspire","install","intact","interest","into","invest","invite","involve","iron","island","isolate","issue","item","ivory","jacket","jaguar","jar","jazz","jealous","jeans","jelly","jewel","job","join","joke","journey","joy","judge","juice","jump","jungle","junior","junk","just","kangaroo","keen","keep","ketchup","key","kick","kid","kidney","kind","kingdom","kiss","kit","kitchen","kite","kitten","kiwi","knee","knife","knock","know","lab","label","labor","ladder","lady","lake","lamp","language","laptop","large","later","latin","laugh","laundry","lava","law","lawn","lawsuit","layer","lazy","leader","leaf","learn","leave","lecture","left","leg","legal","legend","leisure","lemon","lend","length","lens","leopard","lesson","letter","level","liar","liberty","library","license","life","lift","light","like","limb","limit","link","lion","liquid","list","little","live","lizard","load","loan","lobster","local","lock","logic","lonely","long","loop","lottery","loud","lounge","love","loyal","lucky","luggage","lumber","lunar","lunch","luxury","lyrics","machine","mad","magic","magnet","maid","mail","main","major","make","mammal","man","manage","mandate","mango","mansion","manual","maple","marble","march","margin","marine","market","marriage","mask","mass","master","match","material","math","matrix","matter","maximum","maze","meadow","mean","measure","meat","mechanic","medal","media","melody","melt","member","memory","mention","menu","mercy","merge","merit","merry","mesh","message","metal","method","middle","midnight","milk","million","mimic","mind","minimum","minor","minute","miracle","mirror","misery","miss","mistake","mix","mixed","mixture","mobile","model","modify","mom","moment","monitor","monkey","monster","month","moon","moral","more","morning","mosquito","mother","motion","motor","mountain","mouse","move","movie","much","muffin","mule","multiply","muscle","museum","mushroom","music","must","mutual","myself","mystery","myth","naive","name","napkin","narrow","nasty","nation","nature","near","neck","need","negative","neglect","neither","nephew","nerve","nest","net","network","neutral","never","news","next","nice","night","noble","noise","nominee","noodle","normal","north","nose","notable","note","nothing","notice","novel","now","nuclear","number","nurse","nut","oak","obey","object","oblige","obscure","observe","obtain","obvious","occur","ocean","october","odor","off","offer","office","often","oil","okay","old","olive","olympic","omit","once","one","onion","online","only","open","opera","opinion","oppose","option","orange","orbit","orchard","order","ordinary","organ","orient","original","orphan","ostrich","other","outdoor","outer","output","outside","oval","oven","over","own","owner","oxygen","oyster","ozone","pact","paddle","page","pair","palace","palm","panda","panel","panic","panther","paper","parade","parent","park","parrot","party","pass","patch","path","patient","patrol","pattern","pause","pave","payment","peace","peanut","pear","peasant","pelican","pen","penalty","pencil","people","pepper","perfect","permit","person","pet","phone","photo","phrase","physical","piano","picnic","picture","piece","pig","pigeon","pill","pilot","pink","pioneer","pipe","pistol","pitch","pizza","place","planet","plastic","plate","play","please","pledge","pluck","plug","plunge","poem","poet","point","polar","pole","police","pond","pony","pool","popular","portion","position","possible","post","potato","pottery","poverty","powder","power","practice","praise","predict","prefer","prepare","present","pretty","prevent","price","pride","primary","print","priority","prison","private","prize","problem","process","produce","profit","program","project","promote","proof","property","prosper","protect","proud","provide","public","pudding","pull","pulp","pulse","pumpkin","punch","pupil","puppy","purchase","purity","purpose","purse","push","put","puzzle","pyramid","quality","quantum","quarter","question","quick","quit","quiz","quote","rabbit","raccoon","race","rack","radar","radio","rail","rain","raise","rally","ramp","ranch","random","range","rapid","rare","rate","rather","raven","raw","razor","ready","real","reason","rebel","rebuild","recall","receive","recipe","record","recycle","reduce","reflect","reform","refuse","region","regret","regular","reject","relax","release","relief","rely","remain","remember","remind","remove","render","renew","rent","reopen","repair","repeat","replace","report","require","rescue","resemble","resist","resource","response","result","retire","retreat","return","reunion","reveal","review","reward","rhythm","rib","ribbon","rice","rich","ride","ridge","rifle","right","rigid","ring","riot","ripple","risk","ritual","rival","river","road","roast","robot","robust","rocket","romance","roof","rookie","room","rose","rotate","rough","round","route","royal","rubber","rude","rug","rule","run","runway","rural","sad","saddle","sadness","safe","sail","salad","salmon","salon","salt","salute","same","sample","sand","satisfy","satoshi","sauce","sausage","save","say","scale","scan","scare","scatter","scene","scheme","school","science","scissors","scorpion","scout","scrap","screen","script","scrub","sea","search","season","seat","second","secret","section","security","seed","seek","segment","select","sell","seminar","senior","sense","sentence","series","service","session","settle","setup","seven","shadow","shaft","shallow","share","shed","shell","sheriff","shield","shift","shine","ship","shiver","shock","shoe","shoot","shop","short","shoulder","shove","shrimp","shrug","shuffle","shy","sibling","sick","side","siege","sight","sign","silent","silk","silly","silver","similar","simple","since","sing","siren","sister","situate","six","size","skate","sketch","ski","skill","skin","skirt","skull","slab","slam","sleep","slender","slice","slide","slight","slim","slogan","slot","slow","slush","small","smart","smile","smoke","smooth","snack","snake","snap","sniff","snow","soap","soccer","social","sock","soda","soft","solar","soldier","solid","solution","solve","someone","song","soon","sorry","sort","soul","sound","soup","source","south","space","spare","spatial","spawn","speak","special","speed","spell","spend","sphere","spice","spider","spike","spin","spirit","split","spoil","sponsor","spoon","sport","spot","spray","spread","spring","spy","square","squeeze","squirrel","stable","stadium","staff","stage","stairs","stamp","stand","start","state","stay","steak","steel","stem","step","stereo","stick","still","sting","stock","stomach","stone","stool","story","stove","strategy","street","strike","strong","struggle","student","stuff","stumble","style","subject","submit","subway","success","such","sudden","suffer","sugar","suggest","suit","summer","sun","sunny","sunset","super","supply","supreme","sure","surface","surge","surprise","surround","survey","suspect","sustain","swallow","swamp","swap","swarm","swear","sweet","swift","swim","swing","switch","sword","symbol","symptom","syrup","system","table","tackle","tag","tail","talent","talk","tank","tape","target","task","taste","tattoo","taxi","teach","team","tell","ten","tenant","tennis","tent","term","test","text","thank","that","theme","then","theory","there","they","thing","this","thought","three","thrive","throw","thumb","thunder","ticket","tide","tiger","tilt","timber","time","tiny","tip","tired","tissue","title","toast","tobacco","today","toddler","toe","together","toilet","token","tomato","tomorrow","tone","tongue","tonight","tool","tooth","top","topic","topple","torch","tornado","tortoise","toss","total","tourist","toward","tower","town","toy","track","trade","traffic","tragic","train","transfer","trap","trash","travel","tray","treat","tree","trend","trial","tribe","trick","trigger","trim","trip","trophy","trouble","truck","true","truly","trumpet","trust","truth","try","tube","tuition","tumble","tuna","tunnel","turkey","turn","turtle","twelve","twenty","twice","twin","twist","two","type","typical","ugly","umbrella","unable","unaware","uncle","uncover","under","undo","unfair","unfold","unhappy","uniform","unique","unit","universe","unknown","unlock","until","unusual","unveil","update","upgrade","uphold","upon","upper","upset","urban","urge","usage","use","used","useful","useless","usual","utility","vacant","vacuum","vague","valid","valley","valve","van","vanish","vapor","various","vast","vault","vehicle","velvet","vendor","venture","venue","verb","verify","version","very","vessel","veteran","viable","vibrant","vicious","victory","video","view","village","vintage","violin","virtual","virus","visa","visit","visual","vital","vivid","vocal","voice","void","volcano","volume","vote","voyage","wage","wagon","wait","walk","wall","walnut","want","warfare","warm","warrior","wash","wasp","waste","water","wave","way","wealth","weapon","wear","weasel","weather","web","wedding","weekend","weird","welcome","west","wet","whale","what","wheat","wheel","when","where","whip","whisper","wide","width","wife","wild","will","win","window","wine","wing","wink","winner","winter","wire","wisdom","wise","wish","witness","wolf","woman","wonder","wood","wool","word","work","world","worry","worth","wrap","wreck","wrestle","wrist","write","wrong","yard","year","yellow","you","young","youth","zebra","zero","zone","zoo"];
const state = {
  receipt: null,
  batches: [],
  activeBatchIndex: 0,
  activeBatchKey: "",
  nextDepositIndex: 0,
  nextDepositIndexByType: { user: 0, node: 0 },
  depositPurpose: "user",
  selectedNote: 0,
  activeIntentId: null,
  depositOwner: null,
  depositRequestPending: false,
  depositAutoConfirming: false,
  powVisual: null,
  shieldPending: false,
  depositExpiresAt: null,
  depositExpiresAtHeight: null,
  minConfirmations: DEFAULT_MIN_CONFS,
  blocksPerYear: DEFAULT_BLOCKS_PER_YEAR,
  latestBlockHeight: null,
  withdrawingNote: null,
  withdrawnNotes: {},
  publicNoteBuckets: {},
  publicNoteBucketsPending: false,
  shielderSyncCache: null,
  shielderSyncPending: null,
  churnCycleMs: DEFAULT_CHURN_CYCLE_MS,
  churnServerDeltaMs: 0,
  waitStartedAt: null,
  waitMaturesAt: null,
  secretMode: "hidden",
  lastWithdrawalProof: null,
  lastWithdrawalPublic: null,
  lastWithdrawalFeeSats: null,
  lastWithdrawalNoteKey: null,
  lastWithdrawalContextKey: "",
  addressPaneFocusKey: "",
  shieldStageOpenedForDeposit: false,
  withdrawStageOpenedForNotes: "",
  pendingWithdrawNote: null,
  depositDropdownOpen: false,
  openBatchDropdown: "",
  noteRecoveryPending: false,
  noteRecoveryQueued: false,
  noteRecoveryStatus: "idle",
  noteRecoveryBatchKey: "",
  noteRecoveryProgress: null,
  appStarted: false,
  moreOpen: false,
  moreSettled: false,
  quoteWriting: false,
  currentTab: "user",
  nodeConnected: false,
  nodeStatusText: "connecting",
  nodeCount: null,
  routeMode: "local",
  onionAddress: "",
  nodeWorkflow: "new",
  nodeSales: [],
  selectedSaleAuctionId: "",
  paneBatchKeys: {
    deposit: "",
    shield: "",
    withdraw: ""
  },
  e2eWithdrawStarted: false,
  messagePriority: 0,
  messagePriorityUntil: 0
};

const $ = (id) => document.getElementById(id);
let quoteWriteTimer = null;
let moreRevealTimer = null;

function renderNodeEndpoint() {
  const el = $("nodeEndpoint");
  if (!el) {
    return;
  }
  el.textContent = nodeOrigin();
}

function setDot(id, color) {
  const el = $(id);
  if (!el) {
    return;
  }
  el.classList.remove("red", "yellow", "green");
  el.classList.add(color);
}

function renderStatusControls() {
  const isHttps = location.protocol === "https:";
  const isOnion = location.hostname.endsWith(".onion");
  const routeMode = isOnion ? "tor" : isHttps ? "clearnet" : "local";
  state.routeMode = routeMode;
  const routeLabel = $("routeStatusLabel");
  const transportLabel = $("routeTransportLabel");
  if (routeLabel) {
    routeLabel.textContent = isOnion ? "Tor" : isHttps ? "Clearnet" : "Local";
  }
  if (transportLabel) {
    transportLabel.hidden = !isHttps || isOnion;
  }
  setDot("routeStatusDot", isOnion ? "green" : isHttps ? "yellow" : "yellow");

  const switchTor = $("switchTor");
  const hint = $("routeStatusHint");
  if (switchTor) {
    switchTor.disabled = !state.onionAddress;
  }
  if (hint) {
    hint.textContent = state.onionAddress
      ? state.onionAddress
      : "Tor address unavailable";
  }

  const nodeCount = $("nodeCountStatus");
  const nodeLabel = $("nodeStatusLabel");
  if (nodeCount) {
    nodeCount.textContent = Number.isFinite(state.nodeCount) ? String(state.nodeCount) : "loading";
  }
  if (nodeLabel) {
    nodeLabel.textContent = state.nodeConnected ? "Connected" : "Thornado";
  }
  setDot("nodeStatusDot", state.nodeConnected ? "green" : state.nodeStatusText === "connecting" ? "yellow" : "red");
}

async function refreshNodeCount() {
  const nodes = await api("/thornado/nodes");
  const list = Array.isArray(nodes) ? nodes : Array.isArray(nodes?.nodes) ? nodes.nodes : [];
  state.nodeCount = list.filter((node) => String(node?.status || "").toLowerCase() === "active").length || list.length;
  renderStatusControls();
}

function configString(config, ...keys) {
  for (const key of keys) {
    const entry = config?.[key];
    const value = entry?.value ?? entry;
    if (typeof value === "string" && value.trim()) {
      return value.trim();
    }
  }
  return "";
}

function applyRouteConfig(config) {
  state.onionAddress = configString(
    config,
    "ONION_ADDRESS",
    "TOR_ONION_ADDRESS",
    "THORNADO_ONION_ADDRESS",
    "PUBLIC_ONION_ADDRESS",
    "onion_address",
    "tor_onion_address"
  );
  renderStatusControls();
}

function setMessage(text, kind = "", priority = 0, ttlMs = 0) {
  const now = Date.now();
  if (!text) {
    state.messagePriority = 0;
    state.messagePriorityUntil = 0;
  }
  if (priority < state.messagePriority && now < state.messagePriorityUntil) {
    return;
  }
  state.messagePriority = priority;
  state.messagePriorityUntil = ttlMs > 0 ? now + ttlMs : 0;
  const el = $("message");
  el.textContent = text;
  el.className = `message ${kind}`.trim();
}

function log(label, value) {
  const now = new Date().toLocaleTimeString();
  const el = $("log");
  if (!el) {
    return;
  }
  el.textContent = `[${now}] ${label}\n${JSON.stringify(value, null, 2)}\n\n${el.textContent}`;
}

function errorText(error) {
  if (error?.stack) return error.stack;
  if (error?.message) return error.message;
  if (typeof error === "string") return error;
  try {
    return JSON.stringify(error);
  } catch (_) {
    return String(error);
  }
}

function short(value, head = 10, tail = 8) {
  if (!value) {
    return "none";
  }
  return value.length > head + tail ? `${value.slice(0, head)}...${value.slice(-tail)}` : value;
}

function escapeHtml(value) {
  return String(value ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function txHashLink(txid) {
  const value = String(txid || "").trim();
  if (!value) {
    return "none";
  }
  const safe = escapeHtml(value);
  return `<a class="tx-link" href="${btcExplorerUrl(value)}" target="_blank" rel="noopener" title="${safe}">${escapeHtml(short(value, 6, 8))}</a>`;
}

function btcAddressExplorerUrl(address) {
  return `https://mempool.space/testnet/address/${encodeURIComponent(String(address || "").trim())}`;
}

function btcAddressLink(address) {
  const value = String(address || "").trim();
  if (!value) {
    return "No address yet";
  }
  const safe = escapeHtml(value);
  return `<a class="tx-link address-link" href="${btcAddressExplorerUrl(value)}" target="_blank" rel="noopener" title="${safe}">${escapeHtml(short(value, 14, 12))}</a>`;
}

function collapsedDepositSummary(address, hasDeposit, batch = activeBatch()) {
  if (!hasDeposit || !address) {
    return "No address yet";
  }
  const remaining = batchExpiryRemainingMs(batch);
  if (remaining === 0) {
    return `
      <div class="collapsed-address-summary">
        <span>Expired</span>
      </div>
    `;
  }
  const expiry = remaining === null
      ? "Expiry unknown"
      : `Expires in ${formatExpiryHuman(remaining)}`;
  return `
    <div class="collapsed-address-summary">
      ${btcAddressLink(address)}
      <span>${escapeHtml(expiry)}</span>
    </div>
  `;
}

function setStep(id, status) {
  const el = $(id);
  if (!el) {
    return;
  }
  el.classList.remove("active", "done");
  if (status) {
    el.classList.add(status);
  }
}

function setStage(id, status) {
  const el = $(id);
  el.classList.remove("active", "locked", "done");
  if (status) {
    el.classList.add(status);
  }
  const expanded = el.dataset.expanded === "1";
  el.classList.toggle("expanded", expanded);
  const toggle = el.querySelector(".stage-toggle");
  if (toggle) {
    const stageIndex = Array.from(document.querySelectorAll(".stage-card"))
      .filter((stage) => stage.closest(".flow-stage") && !stage.classList.contains("locked"))
      .indexOf(el) + 1;
    toggle.textContent = expanded && stageIndex > 0 ? String(stageIndex).padStart(2, "0") : "+";
    toggle.setAttribute("aria-label", expanded ? "Collapse pane" : "Open pane");
  }
  const wrapper = el.closest(".flow-stage");
  if (wrapper) {
    wrapper.hidden = status === "locked";
  }
}

function toggleStage(stageId) {
  const el = $(stageId);
  const nextExpanded = el.dataset.expanded === "1" ? "0" : "1";
  el.dataset.expanded = nextExpanded;
  if (stageId === "stageDeposit" && nextExpanded === "0") {
    state.addressPaneFocusKey = "";
  }
  if (stageId === "stageWithdraw" && nextExpanded === "0") {
    state.withdrawStageOpenedForNotes = matureNotesKey();
  }
  updateDashboard();
}

function matureNotesKey() {
  return state.batches
    .filter((item) => item.receipt?.notes?.length && batchMature(item))
    .map((item) => `${batchKey(item)}:${item.receipt.notes.length}`)
    .join("|");
}

function renderDepositQr(address) {
  const el = $("depositQr");
  el.textContent = "";
  if (!address) {
    el.className = "qr";
    el.textContent = "Deposit address and QR will appear here";
    return;
  }
  el.className = "qr generated";
  if (typeof qrcode !== "function") {
    el.className = "qr";
    el.textContent = "QR library failed to load";
    return;
  }
  const qr = qrcode(0, "M");
  qr.addData(address);
  qr.make();
  el.innerHTML = qr.createSvgTag(4, 4);
}

function qrMarkup(address) {
  if (!address || typeof qrcode !== "function") {
    return "";
  }
  const qr = qrcode(0, "M");
  qr.addData(address);
  qr.make();
  return qr.createSvgTag(4, 4);
}

function waitRemainingMs() {
  if (!state.waitMaturesAt) {
    return null;
  }
  return Math.max(0, state.waitMaturesAt - Date.now());
}

function batchExpiryRemainingMs(batch = activeBatch()) {
  if (!batch) {
    return null;
  }
  const expiresAt = Number(batch.expiresAt || 0);
  if (Number.isFinite(expiresAt) && expiresAt > 0) {
    return Math.max(0, expiresAt - Date.now());
  }
  const expiryHeight = Number(batch.session?.expires_at_height || batch.deposit?.expires_at_height || 0);
  if (Number.isFinite(expiryHeight) && expiryHeight > 0 && state.latestBlockHeight !== null) {
    return blocksToMs(Math.max(0, expiryHeight - state.latestBlockHeight));
  }
  return null;
}

function depositRemainingMs() {
  const batchRemaining = batchExpiryRemainingMs(activeBatch());
  if (batchRemaining !== null) {
    return batchRemaining;
  }
  if (!state.depositExpiresAt && !state.depositExpiresAtHeight) {
    return null;
  }
  if (state.depositExpiresAt) {
    return Math.max(0, state.depositExpiresAt - Date.now());
  }
  if (state.depositExpiresAtHeight && state.latestBlockHeight !== null) {
    return blocksToMs(Math.max(0, state.depositExpiresAtHeight - state.latestBlockHeight));
  }
  return null;
}

function depositExpired() {
  return state.activeIntentId && (state.depositExpiresAt || state.depositExpiresAtHeight) && depositRemainingMs() === 0;
}

function applyDepositExpiry(...records) {
  const expiryHeight = records
    .map((record) => Number(record?.expires_at_height || 0))
    .find((height) => Number.isFinite(height) && height > 0);
  if (expiryHeight) {
    const heightChanged = state.depositExpiresAtHeight !== expiryHeight;
    state.depositExpiresAtHeight = expiryHeight;
    if (state.latestBlockHeight !== null && (heightChanged || !state.depositExpiresAt)) {
      state.depositExpiresAt = Date.now() + blocksToMs(Math.max(0, expiryHeight - state.latestBlockHeight));
    }
  }
  renderDepositExpiry();
}

function batchTimingFromSession(session = {}, deposit = {}) {
  const createdHeight = Number(session?.created_height || deposit?.created_height || 0);
  const expiresHeight = Number(session?.expires_at_height || deposit?.expires_at_height || 0);
  const currentHeight = state.latestBlockHeight;
  const timing = {};
  if (Number.isFinite(createdHeight) && createdHeight > 0 && currentHeight !== null) {
    timing.issuedAt = Date.now() - blocksToMs(Math.max(0, currentHeight - createdHeight));
  }
  if (Number.isFinite(expiresHeight) && expiresHeight > 0 && currentHeight !== null) {
    timing.expiresAt = Date.now() + blocksToMs(Math.max(0, expiresHeight - currentHeight));
  }
  return timing;
}

function blocksToMs(blocks) {
  const count = Math.max(0, Number(blocks || 0));
  const blocksPerYear = Number(state.blocksPerYear || DEFAULT_BLOCKS_PER_YEAR);
  return Math.round(count * MS_PER_YEAR / Math.max(1, blocksPerYear));
}

function formatClock(ms) {
  if (ms === null) {
    return "--:--:--";
  }
  const total = Math.ceil(ms / 1000);
  const days = String(Math.floor(total / 86400)).padStart(2, "0");
  const hours = String(Math.floor((total % 86400) / 3600)).padStart(2, "0");
  const minutes = String(Math.floor((total % 3600) / 60)).padStart(2, "0");
  return `${days}:${hours}:${minutes}`;
}

function formatWaitClock(ms) {
  if (ms === null) {
    return "--:--:--";
  }
  const total = Math.ceil(Math.max(0, ms) / 1000);
  const days = Math.floor(total / 86400);
  const hours = String(Math.floor((total % 86400) / 3600)).padStart(2, "0");
  const minutes = String(Math.floor((total % 3600) / 60)).padStart(2, "0");
  const seconds = String(total % 60).padStart(2, "0");
  return days > 0 ? `${String(days).padStart(2, "0")}:${hours}:${minutes}:${seconds}` : `${hours}:${minutes}:${seconds}`;
}

function selectedReceiptNote() {
  return state.receipt?.notes?.[state.selectedNote || 0] || null;
}

function noteKey(note) {
  return note?.commitment || `${normalizeDepositType(note?.deposit_type)}:${note?.deposit_index || 0}:${note?.denomination_sats || 0}:${note?.index || 0}`;
}

function isSpentNullifierError(error) {
  return /nullifier already spent/i.test(errorText(error));
}

function normalizeDepositType(depositType) {
  return String(depositType || "user").toLowerCase() === "node" ? "node" : "user";
}

function depositPurposeIndex(depositType) {
  return normalizeDepositType(depositType) === "node" ? 1 : 0;
}

function activeDepositPurpose() {
  return normalizeDepositType(state.depositPurpose);
}

function batchLabel(batch) {
  const type = normalizeDepositType(batch.depositType) === "node" ? "Node" : "User";
  const displayIndex = Number(batch.displayIndex ?? batch.depositOrdinal ?? batch.depositIndex ?? 0);
  return `${type} Deposit ${displayIndex + 1}`;
}

function batchKey(batch) {
  return String(batch?.depositAddress || batch?.owner || batch?.pubkey || `${normalizeDepositType(batch?.depositType)}:${Number(batch?.depositIndex || 0)}`);
}

function combinedReceipt() {
  const notes = state.batches
    .filter((batch) => batch.status === "committed" || batch.receipt?.notes?.length)
    .flatMap((batch) => batch.receipt?.notes || []);
  if (!notes.length) {
    return null;
  }
  return {
    notes,
    remainder_sats: state.batches.reduce((sum, batch) => sum + Number(batch.receipt?.remainder_sats || 0), 0)
  };
}

function setReceiptFromBatches() {
  state.receipt = combinedReceipt();
  state.selectedNote = Math.min(state.selectedNote || 0, Math.max(0, (state.receipt?.notes?.length || 1) - 1));
}

function upsertBatch(batch) {
  const key = batchKey(batch);
  const existingIndex = state.batches.findIndex((item) => batchKey(item) === key);
  if (existingIndex >= 0) {
    const existing = state.batches[existingIndex];
    const keepRecovered = existing.status === "committed" && existing.receipt?.notes?.length && batch.status !== "committed";
    const timing = batchTimingFromSession(batch.session || existing.session, batch.deposit || existing.deposit);
    const depositTxs = mergeDepositTxs(existing.depositTxs, batch.depositTxs);
    state.batches[existingIndex] = {
      ...existing,
      ...batch,
      ...timing,
      issuedAt: batch.issuedAt ?? timing.issuedAt ?? existing.issuedAt,
      expiresAt: batch.expiresAt ?? timing.expiresAt ?? existing.expiresAt,
      amountSats: keepRecovered ? existing.amountSats : batch.amountSats ?? existing.amountSats,
      status: keepRecovered ? existing.status : batch.status ?? existing.status,
      receipt: keepRecovered ? existing.receipt : batch.receipt ?? existing.receipt,
      depositTxs,
      shieldedAt: batch.shieldedAt ?? existing.shieldedAt,
      maturesAt: batch.maturesAt ?? existing.maturesAt
    };
  } else {
    state.batches.push({
      ...batch,
      ...batchTimingFromSession(batch.session, batch.deposit)
    });
  }
  state.batches.sort((a, b) => {
    const typeCmp = normalizeDepositType(a.depositType).localeCompare(normalizeDepositType(b.depositType));
    return typeCmp || Number(a.displayIndex ?? a.depositIndex ?? 0) - Number(b.displayIndex ?? b.depositIndex ?? 0);
  });
  const depositType = normalizeDepositType(batch.depositType);
  state.nextDepositIndexByType[depositType] = Math.max(
    Number(state.nextDepositIndexByType[depositType] || 0),
    Number(batch.depositIndex || 0) + 1
  );
  state.nextDepositIndex = Number(state.nextDepositIndexByType[activeDepositPurpose()] || 0);
  setReceiptFromBatches();
}

function mergeDepositTxs(...groups) {
  const byId = new Map();
  for (const tx of groups.flat().filter(Boolean)) {
    const id = String(tx.txid || "").toUpperCase();
    if (!id) continue;
    byId.set(id, { ...(byId.get(id) || {}), ...tx, txid: id });
  }
  return [...byId.values()].sort((a, b) => String(a.txid).localeCompare(String(b.txid)));
}

function activeBatch() {
  const depositType = activeDepositPurpose();
  return state.batches.find((batch) => batchKey(batch) === String(state.activeBatchKey || "")) ||
    state.batches.find((batch) => normalizeDepositType(batch.depositType) === depositType && Number(batch.depositIndex || 0) === Number(state.activeBatchIndex || 0)) ||
    null;
}

function supersedeOlderIssuedBatches(activeDepositIndex) {
  const activeIndex = Number(activeDepositIndex || 0);
  for (const batch of state.batches) {
    if (Number(batch.depositIndex || 0) < activeIndex && batchIssuedUnexpired(batch)) {
      batch.superseded = true;
    }
  }
}

function activateDepositBatch(batch) {
  if (!batch) {
    return;
  }
  state.activeBatchIndex = Number(batch.depositIndex || 0);
  state.activeBatchKey = batchKey(batch);
  state.depositPurpose = normalizeDepositType(batch.depositType);
  state.depositOwner = batch.owner || null;
  state.activeIntentId = batch.pubkey || batch.depositId || null;
  $("intentId").value = state.activeIntentId || "";
  $("amountSats").value = String(batch.amountSats || 0);
  $("depositAddress").textContent = batch.depositAddress || "";
  applyDepositExpiry(batch.session, batch.deposit);
  const progress = depositConfirmationProgress(batch.session, batch.deposit, batch.txStatus);
  setConfirmationProgress(progress.current, progress.required, progress.seen);
}

function batchConfirmationProgress(batch) {
  const txs = mergeDepositTxs(batch?.depositTxs);
  if (txs.length) {
    const progresses = txs.map((tx) => tx.progress || depositConfirmationProgress(batch?.session, tx.deposit, tx.txStatus));
    const required = Math.max(...progresses.map((item) => Number(item.required || minConfirmations())));
    const current = Math.min(...progresses.map((item) => Number(item.current || 0)));
    return {
      current: Math.max(0, Math.min(required, current)),
      required,
      seen: progresses.some((item) => item.seen)
    };
  }
  return depositConfirmationProgress(batch?.session, batch?.deposit, batch?.txStatus);
}

function batchConfirmed(batch) {
  const progress = batchConfirmationProgress(batch);
  return progress.current >= progress.required
    || batch?.status === "deposit_matched"
    || batch?.status === "committed";
}

function depositFinalised(session = {}, deposit = {}, txStatus = null) {
  const status = String(deposit?.status || session?.status || "");
  const stages = txStatus?.stages || {};
  return status === "deposit_matched"
    || status === "committed"
    || stages.inbound_finalised?.completed === true
    || stages.inbound_confirmation_counted?.completed === true;
}

function batchFinalised(batch) {
  if (batch?.status === "committed" || depositFinalised(batch?.session, batch?.deposit, batch?.txStatus)) {
    return true;
  }
  return mergeDepositTxs(batch?.depositTxs).some((tx) => depositFinalised(batch?.session, tx.deposit, tx.txStatus));
}

function batchMaturityMs(batch) {
  if (!batch?.receipt?.notes?.length) {
    return null;
  }
  const maturesAt = Number(batch.maturesAt || 0);
  if (!Number.isFinite(maturesAt) || maturesAt <= 0) {
    return 0;
  }
  return Math.max(0, maturesAt - Date.now());
}

function batchMature(batch) {
  return batchMaturityMs(batch) === 0;
}

function batchHasDepositValue(batch) {
  return Boolean(
    Number(batch?.amountSats || 0) > 0
      || batch?.inboundTxId
      || batch?.receipt?.notes?.length
  );
}

function requestDepositButtonHidden() {
  if (state.secretMode !== "hidden") {
    return true;
  }
  const batch = activeBatch();
  return Boolean(batch?.depositAddress && !batchExpired(batch));
}

function requestDepositButtonLabel(batch = activeBatch()) {
  if (batch?.depositAddress && batchExpired(batch)) {
    return "Get New Address";
  }
  return "Get Address";
}

function batchExpired(batch) {
  if (!batch?.depositAddress) {
    return false;
  }
  const remaining = batchExpiryRemainingMs(batch);
  return remaining !== null && remaining <= 0;
}

function batchAgeSinceIssued(batch) {
  const issuedAt = Number(batch?.issuedAt || 0);
  if (Number.isFinite(issuedAt) && issuedAt > 0) {
    return formatExpiryHuman(Date.now() - issuedAt);
  }
  const height = Number(batch?.session?.created_height || batch?.deposit?.created_height || 0);
  if (!height || state.latestBlockHeight === null) {
    return "unknown";
  }
  return formatExpiryHuman(blocksToMs(Math.max(0, state.latestBlockHeight - height)));
}

function batchIssuedAge(batch) {
  return batchAgeSinceIssued(batch);
}

function batchExpiredAge(batch) {
  return batchIssuedAge(batch);
}

function batchStatusText(batch) {
  const progress = batchConfirmationProgress(batch);
  if (batch.superseded && !batchHasDepositValue(batch)) {
    return `Expired (${batchIssuedAge(batch)})`;
  }
  if (batchExpired(batch) && !batchHasDepositValue(batch)) {
    return `Expired (${batchExpiredAge(batch)})`;
  }
  if (batchIssuedUnexpired(batch)) {
    return `Issued (${batchIssuedAge(batch)})`;
  }
  return confirmationProgressLabel(progress);
}

function depositLabel(batch) {
  return `Deposit ${Number(batch?.displayIndex ?? batch?.depositOrdinal ?? batch?.depositIndex ?? 0) + 1}`;
}

function normalizeDepositDisplayIndexes() {
  const ordered = [...state.batches].sort((a, b) => {
    const aIssued = Number(a.issuedAt || a.session?.issued_at_unix_ms || a.session?.created_at_unix_ms || 0);
    const bIssued = Number(b.issuedAt || b.session?.issued_at_unix_ms || b.session?.created_at_unix_ms || 0);
    if (aIssued !== bIssued) return aIssued - bIssued;
    const aIndex = Number(a.depositIndex || 0);
    const bIndex = Number(b.depositIndex || 0);
    if (aIndex !== bIndex) return aIndex - bIndex;
    return batchKey(a).localeCompare(batchKey(b));
  });
  ordered.forEach((batch, index) => {
    batch.displayIndex = index;
  });
}

function batchIssuedUnexpired(batch) {
  const status = String(batch?.status || batch?.session?.status || "").toLowerCase();
  return Boolean(
    batch?.depositAddress
      && !batchExpired(batch)
      && !batchHasDepositValue(batch)
      && !batch.superseded
      && (!status || status === "address_issued")
  );
}

function visibleDepositBatches({ includeExpired = false, includeIssued = false } = {}) {
  return state.batches.filter((batch) =>
    batchHasDepositValue(batch)
      || (includeExpired && batchExpired(batch))
      || (includeIssued && batchIssuedUnexpired(batch))
  );
}

function sortedBatches(batches) {
  normalizeDepositDisplayIndexes();
  return [...batches].sort((a, b) => Number(b.displayIndex ?? b.depositIndex ?? 0) - Number(a.displayIndex ?? a.depositIndex ?? 0));
}

function selectedBatchFrom(batches, paneKey = "") {
  const ordered = sortedBatches(batches);
  const selectedKey = paneKey ? state.paneBatchKeys[paneKey] : "";
  return ordered.find((batch) => batchKey(batch) === String(selectedKey || "")) ||
    ordered.find((batch) => batchKey(batch) === String(state.activeBatchKey || "")) ||
    ordered[0] ||
    null;
}

function selectedFlowBatch(batches = visibleDepositBatches({ includeExpired: true, includeIssued: true })) {
  return selectedBatchFrom(batches, "deposit");
}

function renderBatchDropdown(batches, selectedBatch, ariaLabel = "Select deposit", paneKey = "deposit") {
  const ordered = sortedBatches(batches);
  const key = paneKey;
  const dropdown = document.createElement("div");
  dropdown.className = `deposit-batches-dropdown${state.openBatchDropdown === key ? " open" : ""}`;
  dropdown.dataset.paneKey = paneKey;
  const selectedButton = document.createElement("button");
  selectedButton.type = "button";
  selectedButton.className = "deposit-selected";
  selectedButton.dataset.dropdownToggle = paneKey;
  selectedButton.setAttribute("aria-label", ariaLabel);
  selectedButton.setAttribute("aria-expanded", state.openBatchDropdown === key ? "true" : "false");
  selectedButton.innerHTML = `
    <span>${escapeHtml(depositLabel(selectedBatch))}</span>
    <i class="deposit-mark" aria-hidden="true"></i>
  `;
  selectedButton.addEventListener("click", (event) => {
    event.preventDefault();
    event.stopPropagation();
    const shouldOpen = state.openBatchDropdown !== key;
    document.querySelectorAll(".deposit-batches-dropdown.open").forEach((item) => {
      item.classList.remove("open");
      item.querySelector(".deposit-selected")?.setAttribute("aria-expanded", "false");
    });
    state.openBatchDropdown = shouldOpen ? key : "";
    if (shouldOpen) {
      dropdown.classList.add("open");
      selectedButton.setAttribute("aria-expanded", "true");
    } else {
      selectedButton.setAttribute("aria-expanded", "false");
    }
  });
  const menu = document.createElement("div");
  menu.className = "deposit-menu";
  menu.setAttribute("role", "listbox");
  for (const batch of ordered) {
    const option = document.createElement("button");
    option.type = "button";
    option.dataset.dropdownOption = paneKey;
    option.dataset.batchKey = batchKey(batch);
    option.className = `deposit-option${batchKey(batch) === batchKey(selectedBatch) ? " active" : ""}`;
    option.setAttribute("role", "option");
    option.setAttribute("aria-selected", batchKey(batch) === batchKey(selectedBatch) ? "true" : "false");
    const amount = Number(batch.amountSats || batch.receipt?.notes?.reduce((sum, note) => sum + Number(note.denomination_sats || 0), 0) || 0);
    option.innerHTML = `
      <span>${escapeHtml(depositLabel(batch))}${amount > 0 ? ` (${escapeHtml(btcAmount(amount))})` : ""}</span>
      <strong>${escapeHtml(batchStatusText(batch))}</strong>
    `;
    option.addEventListener("click", (event) => {
      event.preventDefault();
      event.stopPropagation();
      state.paneBatchKeys[paneKey] = batchKey(batch);
      if (paneKey === "deposit") {
        activateDepositBatch(batch);
        state.noteRecoveryStatus = "idle";
        state.noteRecoveryBatchKey = batchKey(batch);
        queueDepositRecovery();
      }
      dropdown.classList.remove("open");
      selectedButton.setAttribute("aria-expanded", "false");
      state.openBatchDropdown = "";
      if (paneKey === "deposit") {
        renderDepositHistory();
      }
      updateDashboard();
    });
    menu.append(option);
  }
  dropdown.append(selectedButton, menu);
  return dropdown;
}

function btcExplorerUrl(txHash) {
  return `https://mempool.space/testnet/tx/${String(txHash || "").toLowerCase()}`;
}

function updateNotePoolPosition(note, leavesResponse) {
  const leaves = Array.isArray(leavesResponse.leaves) ? leavesResponse.leaves : [];
  const position = leaves.indexOf(note.commitment);
  note.pool_leaf_position = position >= 0 ? position : null;
  note.pool_leaf_count = Number(leavesResponse.leaf_count || leaves.length);
  note.later_note_count = position >= 0 ? Math.max(0, leaves.length - position - 1) : 0;
  note.privacy_target_later_notes = MIN_LATER_DEPOSITS;
}

function updatePublicNoteBuckets(sync) {
  const buckets = {};
  for (const item of sync?.notes || []) {
    const denomination = Number(item.denomination_sats || 0);
    if (!denomination) {
      continue;
    }
    buckets[denomination] = (buckets[denomination] || 0) + 1;
  }
  state.publicNoteBuckets = buckets;
}

async function shielderLeavesForDenomination(denomination) {
  const sync = await shielderSync();
  const leaves = sync.notesByDenomination.get(Number(denomination || 0)) || [];
  return {
    denomination_sats: Number(denomination || 0),
    leaf_count: leaves.length,
    leaves
  };
}

async function refreshReceiptPoolPositions() {
  const notes = state.receipt?.notes || [];
  if (!notes.length) {
    return;
  }
  const denominations = [...new Set(notes.map((note) => note.denomination_sats))];
  const responses = await Promise.all(
    denominations.map(async (denomination) => [
      denomination,
      await shielderLeavesForDenomination(denomination)
    ])
  );
  const byDenomination = new Map(responses);
  for (const note of notes) {
    const leavesResponse = byDenomination.get(note.denomination_sats);
    if (leavesResponse) {
      updateNotePoolPosition(note, leavesResponse);
    }
  }
}

async function refreshPublicNoteBuckets() {
  if (state.publicNoteBucketsPending) {
    return;
  }
  state.publicNoteBucketsPending = true;
  try {
    await shielderSync({ force: true });
    renderNotes();
  } finally {
    state.publicNoteBucketsPending = false;
  }
}

function findOutboundHash(payload, inHash, recipient = "", amountSats = 0) {
  const txouts = Array.isArray(payload?.txouts)
    ? payload.txouts
    : payload?.keysign
      ? [payload.keysign]
      : payload?.txout
        ? [payload.txout]
        : [];
  for (const txout of txouts) {
    for (const item of txout.tx_array || []) {
      const matchesInHash = inHash && String(item.in_hash || "").toUpperCase() === String(inHash || "").toUpperCase();
      const matchesOutput = recipient
        && item.to_address === recipient
        && Number(item.coin?.amount || 0) === Number(amountSats || 0);
      if ((matchesInHash || matchesOutput) && item.out_hash) {
        return item.out_hash;
      }
    }
  }
  return "";
}

async function scanKeysignBlocksForOutbound(inHash, recipient = "", amountSats = 0, requestedHeight = 0) {
  const current = await api("/thornado/txout");
  const currentHeight = Number(current?.txout?.height || 0);
  const start = Math.max(1, Number(requestedHeight || 0));
  const foundInQueue = findOutboundHash(current, inHash, recipient, amountSats);
  if (foundInQueue || !currentHeight || !start) {
    return foundInQueue;
  }
  const from = Math.max(1, Math.min(start, currentHeight));
  const to = Math.max(from, currentHeight);
  const span = Math.min(250, to - from + 1);
  const rangeStart = to - span + 1;
  const heights = [...new Set([start, ...Array.from({ length: span }, (_, index) => rangeStart + index), currentHeight])]
    .filter((height) => height > 0 && height <= currentHeight);
  for (const height of heights) {
    const payload = await api(`/thornado/keysign/${height}`).catch(() => null);
    const outHash = findOutboundHash(payload, inHash, recipient, amountSats);
    if (outHash) return outHash;
  }
  return "";
}

async function hydrateWithdrawnNotePayouts() {
  const entries = Object.entries(state.withdrawnNotes || {});
  await Promise.all(entries.map(async ([key, withdrawal]) => {
    if (!withdrawal || withdrawal.outHash) {
      return;
    }
    const withdrawalID = withdrawal.withdrawalID || withdrawal.txhash || "";
    if (!withdrawalID) {
      return;
    }
    const redeem = await api(`/thornado/shielder/redeem/${withdrawalID}`).catch(() => null);
    if (!redeem) {
      return;
    }
    const inHash = redeem.in_hash || withdrawalID;
    const recipient = redeem.recipient || "";
    const amountSats = Math.max(0, Number(redeem.amount_sats || 0) - Number(redeem.fee_sats || 0));
    const requestedHeight = Number(redeem.requested_height || 0);
    const outHash = await scanKeysignBlocksForOutbound(inHash, recipient, amountSats, requestedHeight).catch(() => "");
    state.withdrawnNotes[key] = {
      ...withdrawal,
      withdrawalID,
      inHash,
      outHash,
      recipient,
      amountSats,
      status: outHash ? "spent" : withdrawal.status
    };
  }));
}

async function waitForOutboundHash(inHash, recipient = "", amountSats = 0, requestedHeight = 0, timeoutMs = 900000) {
  const started = Date.now();
  while (Date.now() - started < timeoutMs) {
    const outHash = await scanKeysignBlocksForOutbound(inHash, recipient, amountSats, requestedHeight);
    if (outHash) {
      return outHash;
    }
    await new Promise((resolve) => setTimeout(resolve, 3000));
  }
  return "";
}

function formatDuration(ms) {
  if (ms === null) {
    return "not scheduled";
  }
  if (ms <= 0) {
    return "expired";
  }
  const totalMinutes = Math.ceil(ms / 60000);
  const days = Math.floor(totalMinutes / 1440);
  const hours = Math.floor((totalMinutes % 1440) / 60);
  const minutes = totalMinutes % 60;
  const parts = [];
  if (days) {
    parts.push(`${days}d`);
  }
  if (hours || days) {
    parts.push(`${hours}h`);
  }
  parts.push(`${minutes}m`);
  return parts.join(" ");
}

function formatChurnCountdown(ms) {
  if (ms === null) {
    return "--:--:--:--";
  }
  const totalSeconds = Math.ceil(Math.max(0, ms) / 1000);
  const days = String(Math.floor(totalSeconds / 86400)).padStart(2, "0");
  const hours = String(Math.floor((totalSeconds % 86400) / 3600)).padStart(2, "0");
  const minutes = String(Math.floor((totalSeconds % 3600) / 60)).padStart(2, "0");
  const seconds = String(totalSeconds % 60).padStart(2, "0");
  return `${days}:${hours}:${minutes}:${seconds}`;
}

function formatExpiryHuman(ms) {
  if (ms === null) {
    return "not scheduled";
  }
  const totalSeconds = Math.ceil(Math.max(0, ms) / 1000);
  if (totalSeconds < 60) {
    return `${totalSeconds} sec`;
  }
  const totalMinutes = Math.ceil(totalSeconds / 60);
  if (totalMinutes < 60) {
    return `${totalMinutes} min`;
  }
  const totalHours = Math.ceil(totalMinutes / 60);
  if (totalHours < 24) {
    return `${totalHours} ${totalHours === 1 ? "hour" : "hours"}`;
  }
  const totalDays = Math.ceil(totalHours / 24);
  return `${totalDays} ${totalDays === 1 ? "day" : "days"}`;
}

function applyChurnWindow(window) {
  state.churnCycleMs = window.cycle_ms || DEFAULT_CHURN_CYCLE_MS;
  state.churnServerDeltaMs = Date.now() - window.server_now_ms;
  if (window.remaining_ms !== undefined) {
    state.depositExpiresAt = Date.now() + window.remaining_ms;
  } else {
    state.depositExpiresAt = window.next_churn_at_ms + state.churnServerDeltaMs;
  }
  const intervalEl = $("networkChurnInterval");
  if (intervalEl) {
    intervalEl.textContent = formatDuration(state.churnCycleMs);
  }
  const targetNodesEl = $("networkTargetNodes");
  if (targetNodesEl && window.target_active_nodes !== undefined) {
    targetNodesEl.textContent = String(window.target_active_nodes);
  }
  const maxChurnEl = $("networkMaxChurn");
  if (maxChurnEl && window.max_nodes_per_churn !== undefined) {
    maxChurnEl.textContent = `${window.max_nodes_per_churn} per cycle`;
  }
  renderDepositExpiry();
  return window;
}

async function refreshChurnWindow() {
  const config = await api("/thornado/config").catch((error) => {
    log("thornado/config", { error: error.message });
    return null;
  });
  applyRouteConfig(config);
  applyBlockTimingConfig(config);
  applyDepositConfirmationConfig(config);
  const now = Date.now();
  return applyChurnWindow({
    server_time_ms: now,
    next_churn_at_ms: now + DEFAULT_CHURN_CYCLE_MS,
    cycle_ms: DEFAULT_CHURN_CYCLE_MS,
    max_nodes_per_churn: 0
  });
}

function configNumber(config, ...keys) {
  for (const key of keys) {
    const entry = config?.[key];
    const value = Number(entry?.value ?? entry);
    if (Number.isFinite(value) && value > 0) {
      return value;
    }
  }
  return null;
}

function minConfirmations() {
  return Math.max(1, Number(state.minConfirmations || DEFAULT_MIN_CONFS) || DEFAULT_MIN_CONFS);
}

function applyDepositConfirmationConfig(config) {
  const value = configNumber(config, "BTC_ConfirmationsMin", "BTC_CONFIRMATIONSMIN", "BTC_CONFIRMATIONS_MIN");
  if (value !== null) {
    state.minConfirmations = Math.max(1, value);
  }
}

function applyBlockTimingConfig(config) {
  const blocksPerYear = configNumber(config, "BLOCKS_PER_YEAR", "BLOCKSPERYEAR", "MINT_BLOCKSPERYEAR", "MINT_BLOCKS_PER_YEAR");
  if (blocksPerYear) {
    state.blocksPerYear = blocksPerYear;
    return;
  }
  const blockTimeSeconds = configNumber(config, "CHAIN_BLOCKTIMESECONDS", "CHAIN_BLOCK_TIME_SECONDS");
  if (blockTimeSeconds) {
    state.blocksPerYear = Math.round((365 * 24 * 60 * 60) / blockTimeSeconds);
  }
}

function renderDepositExpiry() {
  const el = $("depositExpiry");
  const nextChurnEl = $("networkNextChurn");
  if (!state.depositExpiresAt && !state.depositExpiresAtHeight) {
    el.hidden = true;
    if (nextChurnEl) {
      nextChurnEl.textContent = "not scheduled";
    }
    return;
  }
  const remaining = depositRemainingMs();
  const expired = remaining === 0;
  if (nextChurnEl) {
    nextChurnEl.textContent = expired ? "expired" : formatChurnCountdown(remaining);
  }
  if (!state.activeIntentId) {
    el.hidden = true;
    return;
  }
  el.hidden = false;
  el.className = `expiry-line${expired ? " expired" : ""}`;
  if (expired) {
    el.innerHTML = `<strong>Deposit address expired</strong><span>Request a new address.</span>`;
    return;
  }
  el.innerHTML = `<strong>Address expires in ${formatExpiryHuman(remaining)}</strong>`;
}

function renderSecret() {
  const secret = $("walletRoot").value.trim();
  const isRevealed = state.secretMode === "revealed";
  const isCustom = state.secretMode === "custom";
  $("stageDeposit").classList.toggle("secret-focus", isRevealed || isCustom);
  $("customSecretPanel").hidden = !isCustom;
  $("secretBox").hidden = isCustom;
  $("revealSecret").hidden = isRevealed;
  $("secretLine").hidden = !isRevealed;
  $("secretHelp").hidden = !isRevealed;
  $("secretActions").hidden = !isRevealed;
  $("secretValue").textContent = isRevealed ? secret : "";
  $("copySecret").textContent = "Copy";
  $("requestDeposit").hidden = requestDepositButtonHidden();
  $("requestDeposit").textContent = requestDepositButtonLabel();
}

function resetDepositState() {
  state.activeIntentId = null;
  state.depositOwner = null;
  state.batches = [];
  state.activeBatchIndex = 0;
  state.nextDepositIndex = 0;
  state.depositExpiresAt = null;
  state.depositExpiresAtHeight = null;
  state.churnCycleMs = DEFAULT_CHURN_CYCLE_MS;
  state.receipt = null;
  state.selectedNote = 0;
  state.waitStartedAt = null;
  state.waitMaturesAt = null;
  state.lastWithdrawalProof = null;
  state.lastWithdrawalPublic = null;
  state.lastWithdrawalFeeSats = null;
  state.lastWithdrawalNoteKey = null;
  state.lastWithdrawalContextKey = "";
  state.shieldStageOpenedForDeposit = false;
  state.shieldPending = false;
  state.withdrawingNote = null;
  state.withdrawnNotes = {};
  state.depositDropdownOpen = false;
  state.openBatchDropdown = "";
  state.noteRecoveryPending = false;
  state.noteRecoveryQueued = false;
  state.noteRecoveryStatus = "idle";
  state.noteRecoveryBatchKey = "";
  state.noteRecoveryProgress = null;
  state.paneBatchKeys = { deposit: "", shield: "", withdraw: "" };
  ["stageDeposit", "stageDepositTrack", "stageShield", "stageWait", "stageWithdraw"].forEach((id) => {
    $(id).dataset.expanded = "0";
  });
  $("intentId").value = "";
  $("depositAddress").hidden = true;
  $("depositResult").hidden = true;
  $("clientPubkey").hidden = true;
  $("powResult").hidden = true;
  $("confirmations").textContent = `0 / ${minConfirmations()}`;
  renderDepositHistory();
}

function renderDepositHistory() {
  const el = $("depositBatchList");
  if (!el) {
    return;
  }
  el.textContent = "";
  const batches = visibleDepositBatches({ includeExpired: true, includeIssued: true });
  if (!batches.length) {
    return;
  }
  const orderedBatches = sortedBatches(batches);
  if (!orderedBatches.some((batch) => batchKey(batch) === String(state.paneBatchKeys.deposit || ""))) {
    state.paneBatchKeys.deposit = batchKey(orderedBatches[0]);
  }
  const selectedBatch = selectedBatchFrom(orderedBatches, "deposit");
  const dropdown = document.createElement("div");
  dropdown.className = "batch-card";
  const progress = batchConfirmationProgress(selectedBatch);
  const txid = selectedBatch.inboundTxId || selectedBatch.depositId || "";
  const detail = document.createElement("div");
  detail.className = "deposit-batch-row";
  const detailRows = [];
  const txRows = mergeDepositTxs(selectedBatch.depositTxs);
  if (txRows.length) {
    for (const tx of txRows) {
      const txProgress = tx.progress || depositConfirmationProgress(selectedBatch.session, tx.deposit, tx.txStatus);
      detailRows.push(`
        <div class="row">
          <span>${txHashLink(tx.txid)}</span>
          <strong>${btcAmount(Number(tx.amountSats || 0))} · ${confirmationProgressLabel(txProgress)}</strong>
        </div>
      `);
    }
  } else if (txid) {
    detailRows.push(`<div class="row"><span>Tx ID</span><strong>${txHashLink(txid)}</strong></div>`);
  }
  if (batchHasDepositValue(selectedBatch)) {
    detailRows.push(`<div class="row"><span>Total</span><strong>${btcAmount(Number(selectedBatch.amountSats || 0))}</strong></div>`);
  }
  if (!txRows.length && (!batchExpired(selectedBatch) || batchHasDepositValue(selectedBatch))) {
    detailRows.push(`<div class="row"><span>Confirmations</span><strong>${confirmationProgressLabel(progress)}</strong></div>`);
  }
  detail.innerHTML = detailRows.join("");
  dropdown.append(renderBatchDropdown(orderedBatches, selectedBatch, "Select tracked deposit", "deposit"));
  el.append(dropdown, detail);
}

function hasRecoverableDepositBatch() {
  return state.batches.some((batch) => batchConfirmed(batch) || batchFinalised(batch) || String(batch?.status || "").toLowerCase() === "committed");
}

function noteRecoveryInPhase(phase) {
  return state.noteRecoveryStatus === phase && Boolean(state.noteRecoveryProgress);
}

function noteRecoveryVisible() {
  return Boolean(state.noteRecoveryProgress)
    && ["searching_commitments", "searching_nullifiers", "done"].includes(state.noteRecoveryStatus);
}

function noteRecoveryLabel() {
  if (state.noteRecoveryStatus === "searching_nullifiers") {
    return "Matching notes";
  }
  if (state.noteRecoveryStatus === "done") {
    return "Sync complete";
  }
  return "Syncing public set";
}

function renderDenominations() {
  const el = $("denomBreakdown");
  el.textContent = "";
  const counts = {};
  for (const note of state.receipt?.notes || []) {
    counts[note.denomination_sats] = (counts[note.denomination_sats] || 0) + 1;
  }
  if (!Object.keys(counts).length) {
    const row = document.createElement("div");
    row.className = "row";
    row.innerHTML = "<span>No notes minted</span><strong>shield pending</strong>";
    el.append(row);
    return;
  }
  for (const denomination of Object.keys(counts).sort((a, b) => Number(b) - Number(a))) {
    const row = document.createElement("div");
    row.className = "row";
    row.innerHTML = `<span>${btcAmount(Number(denomination))}</span><strong>${counts[denomination]} notes</strong>`;
    el.append(row);
  }
}

function hydrateReceipt() {
  setReceiptFromBatches();
  if (!state.receipt?.notes?.length) {
    return;
  }
  const maturedAt = Date.now() - 1;
  for (const batch of state.batches) {
    if (batch.receipt?.notes?.length && !batch.maturesAt) {
      batch.shieldedAt = batch.shieldedAt || maturedAt - DEMO_MATURITY_MS;
      batch.maturesAt = maturedAt;
    }
  }
  state.waitStartedAt = state.waitStartedAt || maturedAt - DEMO_MATURITY_MS;
  state.waitMaturesAt = state.waitMaturesAt || maturedAt;
  state.selectedNote = Math.min(state.selectedNote || 0, state.receipt.notes.length - 1);
  $("confirmations").textContent = `${minConfirmations()} / ${minConfirmations()}`;
  $("amountSats").value = String(state.batches.reduce((sum, batch) => sum + Number(batch.amountSats || 0), 0));
  $("depositResult").hidden = true;
  $("depositQr").hidden = true;
  $("depositAddress").hidden = true;
  $("copyDepositAddress").hidden = true;
  $("stageDeposit").dataset.expanded = "0";
  $("stageShield").dataset.expanded = "0";
  $("stageWait").dataset.expanded = "0";
  $("stageWithdraw").dataset.expanded = "1";
}

function openWithdrawForRecoveredNotes() {
  if (!state.receipt?.notes?.length) {
    return;
  }
  $("stageDeposit").dataset.expanded = "0";
  $("stageShield").dataset.expanded = "0";
  $("stageWait").dataset.expanded = "0";
  $("stageWithdraw").dataset.expanded = "1";
}

function updateDashboard() {
  setReceiptFromBatches();
  renderIntroGate();
  const depositDropdown = document.querySelector("#stageDepositTrack .deposit-batches-dropdown");
  const depositDropdownBusy = state.openBatchDropdown === "deposit"
    || Boolean(depositDropdown?.matches(":hover"))
    || Boolean(depositDropdown?.contains(document.activeElement));
  if (!depositDropdownBusy) {
    renderDepositHistory();
  }
  const notes = state.receipt?.notes || [];
  const displayBatches = visibleDepositBatches({ includeIssued: true });
  const batch = activeBatch();
  const hasWallet = Boolean($("walletRoot").value.trim());
  const activeDepositAddress = $("depositAddress").textContent.trim() || batch?.depositAddress || "";
  const activeDepositIntent = state.activeIntentId || batch?.pubkey || batch?.depositId || "";
  const hasDeposit = Boolean(activeDepositIntent && activeDepositAddress && !state.depositRequestPending);
  const isDepositExpired = depositExpired();
  const hasObservableDeposit = hasDeposit && !isDepositExpired;
  const confirmationProgress = confirmationProgressFromUi();
  const hasSeenDeposit = confirmationProgress.seen || batch?.status === "deposit_observed";
  const hasConfirmed = confirmationProgress.current >= confirmationProgress.required
    || batch?.status === "deposit_matched"
    || batch?.status === "committed";
  const anyConfirmed = state.batches.some((item) => batchConfirmed(item)) || hasConfirmed;
  const selectedFinalised = batchFinalised(batch);
  const selectedNotes = batch?.receipt?.notes || [];
  const hasShielded = selectedNotes.length > 0;
  const hasMatureNote = hasShielded && batchMature(batch);
  const selectedBatchKey = batch ? batchKey(batch) : "";
  const selectedMatureNoteKey = hasMatureNote ? `${selectedBatchKey}:${selectedNotes.length}` : "";
  const addressFocusActive = Boolean(
    state.depositRequestPending ||
    (
      state.addressPaneFocusKey &&
      state.addressPaneFocusKey === selectedBatchKey &&
      hasDeposit &&
      !hasSeenDeposit &&
      !anyConfirmed &&
      !isDepositExpired
    )
  );
  if (!addressFocusActive && selectedFinalised && state.shieldStageOpenedForDeposit !== selectedBatchKey) {
    $("stageDeposit").dataset.expanded = "0";
    $("stageDepositTrack").dataset.expanded = "1";
    $("stageShield").dataset.expanded = "1";
    state.shieldStageOpenedForDeposit = selectedBatchKey;
  }
  if (!addressFocusActive && hasMatureNote && selectedMatureNoteKey && state.withdrawStageOpenedForNotes !== selectedMatureNoteKey) {
    $("stageDeposit").dataset.expanded = "0";
    $("stageShield").dataset.expanded = "0";
    $("stageWait").dataset.expanded = "0";
    $("stageWithdraw").dataset.expanded = "1";
    state.withdrawStageOpenedForNotes = selectedMatureNoteKey;
  }
  const hasTrackedDeposits = displayBatches.length > 0;
  const hasUnconfirmedSeenDeposit = displayBatches.some((item) => {
    const progress = batchConfirmationProgress(item);
    return progress.seen && progress.current < progress.required;
  });
  const showDepositTracking = hasDeposit || hasTrackedDeposits || hasUnconfirmedSeenDeposit;
  if (!hasDeposit && !hasTrackedDeposits && !state.depositRequestPending) {
    $("stageDeposit").dataset.expanded = "1";
  }
  if (hasDeposit && !hasSeenDeposit && !anyConfirmed && !isDepositExpired) {
    $("stageDeposit").dataset.expanded = "1";
    $("stageDepositTrack").dataset.expanded = "1";
  }
  const hasProof = Boolean(state.lastWithdrawalProof && state.lastWithdrawalPublic);
  const note = selectedNotes[state.selectedNote || 0] || selectedNotes[0] || null;
  const laterNoteCount = note?.later_note_count;
  const hasLaterNoteCount = Number.isFinite(laterNoteCount);
  const targetLaterNotes = Number.isFinite(note?.privacy_target_later_notes)
    ? note.privacy_target_later_notes
    : MIN_LATER_DEPOSITS;
  const laterNoteProgress = hasLaterNoteCount
    ? `${laterNoteCount} / ${targetLaterNotes}`
    : hasShielded ? "syncing" : "none";
  const missingLaterNotes = hasLaterNoteCount
    ? Math.max(0, targetLaterNotes - laterNoteCount)
    : targetLaterNotes;
  const poolReady = hasMatureNote && hasLaterNoteCount && laterNoteCount >= targetLaterNotes;
  $("walletMetric").textContent = hasWallet ? "Mnemonic local" : "No mnemonic";
  $("depositMetric").textContent = displayBatches.length
    ? `${displayBatches.length} batches`
    : hasDeposit ? isDepositExpired ? "Expired" : state.activeIntentId : "No intent";
  $("notesMetric").textContent = `${selectedNotes.length} minted`;
  $("privacyMetric").textContent = hasShielded ? `${laterNoteProgress} later` : "No pool yet";
  setStep("stepWallet", hasWallet ? "done" : "active");
  setStep("stepDeposit", hasObservableDeposit || anyConfirmed ? "done" : hasWallet ? "active" : "");
  setStep("stepShield", hasShielded ? "done" : selectedFinalised ? "active" : "");
  setStep("stepWithdraw", hasShielded ? "active" : "");
  setStage("stageDeposit", hasDeposit && (hasSeenDeposit || anyConfirmed) ? "done" : "active");
  setStage("stageDepositTrack", showDepositTracking ? (anyConfirmed ? "done" : "active") : "locked");
  setStage("stageShield", hasShielded ? "done" : selectedFinalised ? "active" : "locked");
  const waitWrapper = $("stageWait")?.closest(".flow-stage");
  if (waitWrapper) {
    waitWrapper.hidden = true;
  }
  setStage("stageWithdraw", hasMatureNote ? "active" : "locked");
  $("shieldDeposit").disabled = !selectedFinalised || batch?.status === "committed" || state.shieldPending;
  $("shieldDeposit").innerHTML = state.shieldPending
    ? '<span class="button-spinner" aria-hidden="true"></span>Shielding'
    : "Shield Deposit";
  $("withdrawNote").disabled = !hasMatureNote;
  $("validateProof").disabled = !hasMatureNote;
  $("depositResult").hidden = !hasDeposit;
  $("depositAddressLabel").hidden = !hasDeposit || isDepositExpired;
  $("depositQr").hidden = !hasDeposit || isDepositExpired;
  $("depositAddress").hidden = !hasDeposit || isDepositExpired;
  $("copyDepositAddress").hidden = !hasDeposit || isDepositExpired;
  $("requestDeposit").hidden = requestDepositButtonHidden();
  $("requestDeposit").textContent = requestDepositButtonLabel(batch);
  if (batch?.depositAddress && !$("depositAddress").textContent.trim()) {
    $("depositAddress").textContent = batch.depositAddress;
  }
  renderDepositQr(hasDeposit && !isDepositExpired ? activeDepositAddress : "");
  renderDepositExpiry();
  $("knownAmount").textContent = btcAmount(Number($("amountSats").value || 0));
  const activeDepositId = batch?.inboundTxId || batch?.depositId || $("intentId").value.trim();
  $("shieldDepositDetails").hidden = true;
  $("shieldDepositAmount").textContent = btcAmount(Number(batch?.amountSats || $("amountSats").value || 0));
  $("shieldDepositTxid").innerHTML = txHashLink(activeDepositId);
  $("shieldDepositTxid").title = activeDepositId || "";
  $("shieldDeposit").hidden = true;
  $("denomBreakdown").hidden = true;
  $("laterDepositMetric").textContent = laterNoteProgress;
  $("withdrawRisk").textContent = poolReady
    ? "lower after later notes"
    : hasMatureNote && hasLaterNoteCount
      ? `wait for ${missingLaterNotes} more same-denom notes`
      : hasShielded
        ? hasMatureNote ? "syncing pool position" : "high until time passes"
        : "high before shield";
  const poolProgress = hasLaterNoteCount
    ? Math.min(1, laterNoteCount / targetLaterNotes)
    : 0;
  $("privacyMeter").style.width = hasShielded
    ? `${Math.round(30 + poolProgress * 62)}%`
    : hasDeposit ? "18%" : "8%";
  $("receiptFingerprint").textContent = selectedNotes[0]?.root_fingerprint || "none";
  $("remainderSats").textContent = btcAmount(state.receipt?.remainder_sats || 0);
  $("feePreview").textContent = btcAmount(Number($("feeSats").value || 0));
  $("depositSummary").innerHTML = collapsedDepositSummary(activeDepositAddress, hasDeposit, batch);
  $("depositTrackSummary").textContent = displayBatches.length
    ? `${displayBatches.length} deposits tracked`
    : hasUnconfirmedSeenDeposit
      ? "Deposit seen"
      : hasDeposit
        ? "Watching current address"
      : "No deposits tracked";
  $("shieldSummary").textContent = hasShielded
    ? `${selectedNotes.length} notes minted`
    : selectedFinalised
      ? `Matched ${short(state.activeIntentId, 12, 10)}`
      : "Waiting for deposit";
  const withdrawableNoteCount = hasMatureNote
    ? selectedNotes.filter((item) => !item.spent && !state.withdrawnNotes[noteKey(item)]).length
    : 0;
  $("withdrawSummary").textContent = hasProof
    ? "Proof generated"
    : hasMatureNote
      ? `${withdrawableNoteCount} notes available`
      : "Waiting for maturity";
  renderDenominations();
  renderShieldBatches();
  renderNotes();
  renderSecret();
}

async function api(path, options = {}) {
  try {
    return await requestJson(path, options);
  } catch (error) {
    let message = error.message || String(error);
    if (message === "deposit intent not found") {
      message = "Deposit intent not found on this node. Request a fresh deposit address, then observe that new intent.";
    }
    throw new Error(message);
  }
}

function invalidateShielderSyncCache() {
  state.shielderSyncCache = null;
  state.shielderSyncPending = null;
}

function setNoteRecoveryProgress(progress = null) {
  state.noteRecoveryProgress = progress;
  updateDashboard();
}

function pageCursor(page, stream) {
  return page?.[`next_${stream}_cursor`]
    || page?.next_cursors?.[stream]
    || page?.cursors?.[stream]
    || "";
}

function pageTotal(page, stream, rows) {
  const value = page?.[`total_${stream}s`]
    ?? page?.totals?.[stream]
    ?? page?.totals?.[`${stream}s`]
    ?? null;
  const total = Number(value);
  return Number.isFinite(total) && total >= 0 ? total : rows.length;
}

function stablePublicKey(row, fields, fallbackPrefix) {
  for (const field of fields) {
    const value = String(row?.[field] || "").trim();
    if (value) {
      return `${field}:${value}`;
    }
  }
  return `${fallbackPrefix}:${JSON.stringify(row || {})}`;
}

function uniquePublicRows(rows, fields, fallbackPrefix) {
  const seen = new Set();
  const out = [];
  for (const row of rows || []) {
    const key = stablePublicKey(row, fields, fallbackPrefix);
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    out.push(row);
  }
  return out;
}

function noteOrderValue(note) {
  const value = Number(
    note?.leaf_index
    ?? note?.leaf_position
    ?? note?.merkle_index
    ?? note?.index
    ?? NaN
  );
  return Number.isFinite(value) ? value : Number.MAX_SAFE_INTEGER;
}

function shielderSyncHasMore(page, cursors) {
  if (typeof page?.has_more === "boolean") {
    return page.has_more;
  }
  if (page?.has_more && typeof page.has_more === "object") {
    return Boolean(page.has_more.deposit || page.has_more.deposits || page.has_more.note || page.has_more.notes || page.has_more.nullifier || page.has_more.nullifiers);
  }
  return Boolean(cursors.deposit || cursors.note || cursors.nullifier);
}

async function shielderSync(options = {}) {
  const force = Boolean(options.force);
  const onProgress = typeof options.onProgress === "function" ? options.onProgress : null;
  const now = Date.now();
  if (!force && state.shielderSyncCache && now - state.shielderSyncCache.fetchedAt < POOL_REFRESH_MS) {
    if (onProgress) {
      onProgress({ percent: 100, done: true, ...state.shielderSyncCache.stats });
    }
    return state.shielderSyncCache.payload;
  }
  if (!force && state.shielderSyncPending) {
    return state.shielderSyncPending;
  }
  state.shielderSyncPending = (async () => {
    const payload = { notes: [], nullifiers: [], deposits: [] };
    const cursors = { deposit: "", note: "", nullifier: "" };
    let stats = { loaded: 0, total: 0, percent: 0 };
    let hasMore = true;
    do {
      const params = new URLSearchParams({ limit: String(SHIELDER_SYNC_PAGE_LIMIT) });
      if (cursors.deposit) params.set("deposit_cursor", cursors.deposit);
      if (cursors.note) params.set("note_cursor", cursors.note);
      if (cursors.nullifier) params.set("nullifier_cursor", cursors.nullifier);
      const page = await api(`/thornado/shielder/sync?${params.toString()}`);
      payload.notes.push(...(page.notes || []));
      payload.nullifiers.push(...(page.nullifiers || []));
      payload.deposits.push(...(page.deposits || []));
      cursors.deposit = pageCursor(page, "deposit");
      cursors.note = pageCursor(page, "note");
      cursors.nullifier = pageCursor(page, "nullifier");
      hasMore = shielderSyncHasMore(page, cursors);
      if (hasMore && !cursors.deposit && !cursors.note && !cursors.nullifier) {
        hasMore = false;
      }
      const total = pageTotal(page, "deposit", payload.deposits)
        + pageTotal(page, "note", payload.notes)
        + pageTotal(page, "nullifier", payload.nullifiers);
      const loaded = payload.deposits.length + payload.notes.length + payload.nullifiers.length;
      stats = {
        phase: "public",
        loaded,
        total,
        percent: hasMore && total > 0 ? Math.min(99, Math.floor((loaded / total) * 100)) : 100,
        deposits: payload.deposits.length,
        notes: payload.notes.length,
        nullifiers: payload.nullifiers.length
      };
      if (onProgress) {
        onProgress(stats);
      }
    } while (hasMore);
    stats = { ...stats, phase: "public", percent: 100, done: true };
    if (onProgress) {
      onProgress(stats);
    }
    payload.notes = uniquePublicRows(payload.notes, ["commitment", "note_id"], "note");
    payload.nullifiers = uniquePublicRows(payload.nullifiers, ["nullifier_hash"], "nullifier");
    payload.deposits = uniquePublicRows(payload.deposits, ["deposit_id", "txid", "tx_id"], "deposit");
    payload.notes.sort((a, b) => {
      const denomA = Number(a.denomination_sats || 0);
      const denomB = Number(b.denomination_sats || 0);
      if (denomA === denomB) {
        const orderA = noteOrderValue(a);
        const orderB = noteOrderValue(b);
        if (orderA !== orderB) {
          return orderA - orderB;
        }
        return String(a.commitment || "").localeCompare(String(b.commitment || ""));
      }
      return denomA - denomB;
    });
    payload.nullifiers.sort((a, b) => String(a.nullifier_hash || "").localeCompare(String(b.nullifier_hash || "")));
    payload.deposits.sort((a, b) => String(a.deposit_id || "").localeCompare(String(b.deposit_id || "")));
      const notesByDenomination = new Map();
      for (const note of payload?.notes || []) {
        const denomination = Number(note.denomination_sats || 0);
        if (!denomination || !note.commitment) {
          continue;
        }
        const leaves = notesByDenomination.get(denomination) || [];
        leaves.push(note.commitment);
        notesByDenomination.set(denomination, leaves);
      }
      const normalized = {
        notes: payload?.notes || [],
        nullifiers: payload?.nullifiers || [],
        deposits: payload?.deposits || [],
        notesByDenomination,
        nullifierSet: new Set((payload?.nullifiers || []).map((item) => String(item.nullifier_hash || "").trim()).filter(Boolean))
      };
      state.shielderSyncCache = { fetchedAt: Date.now(), payload: normalized, stats };
      updatePublicNoteBuckets(normalized);
      return normalized;
  })().finally(() => {
      state.shielderSyncPending = null;
    });
  return state.shielderSyncPending;
}

async function refreshHash() {
  let payload;
  try {
    payload = await api("/thornado/block");
  } catch (error) {
    state.nodeConnected = false;
    state.nodeStatusText = "disconnected";
    renderStatusControls();
    throw error;
  }
  const height = payload?.id?.height || payload?.header?.height || payload?.block?.header?.height || "latest";
  const producer = payload?.header?.proposer_address || payload?.block?.header?.proposer_address || "unknown";
  const numericHeight = Number(height);
  if (Number.isFinite(numericHeight)) {
    state.latestBlockHeight = numericHeight;
  }
  state.nodeConnected = true;
  state.nodeStatusText = "connected";
  $("blockHeight").textContent = String(height);
  $("blockProducer").textContent = short(producer, 10, 8);
  $("blockProducer").title = producer;
  renderStatusControls();
  renderDepositExpiry();
}

function sats(value) {
  return new Intl.NumberFormat().format(value);
}

function btcAmount(value) {
  const btc = Number(value || 0) / 100000000;
  const text = btc === 0
    ? "0"
    : btc >= 1
      ? btc.toFixed(8).replace(/0+$/, "").replace(/\.$/, "")
      : btc.toFixed(8).replace(/0+$/, "").replace(/\.$/, "");
  return `${text} BTC`;
}

function getAmountSats() {
  const amount = Number($("amountSats").value);
  if (!Number.isSafeInteger(amount) || amount < 0) {
    throw new Error("amount must be a positive BTC value");
  }
  return amount;
}

function getIntentId() {
  const id = $("intentId").value.trim() || state.activeIntentId || activeBatch()?.pubkey || activeBatch()?.depositId || "";
  if (!id) {
    throw new Error("intent ID is required");
  }
  return id;
}

function greedyDenominations(amountSats) {
  let remaining = amountSats;
  const denominations = [];
  for (const denomination of DENOMINATIONS) {
    if (denomination < SHIELDER_NOTE_MIN_SATS) {
      continue;
    }
    while (remaining >= denomination) {
      denominations.push(denomination);
      remaining -= denomination;
    }
  }
  return { denominations, remaining };
}

function withdrawalFeeForDenomination(denominationSats) {
  return Math.floor(Number(denominationSats || 0) * WITHDRAWAL_FEE_BASIS_POINTS / 10000);
}

function applySpendableNoteFloor(receipt) {
  const notes = Array.isArray(receipt?.notes) ? receipt.notes : [];
  let feeRemainder = Number(receipt?.remainder_sats || 0);
  const filtered = [];
  for (const note of notes) {
    const denomination = Number(note.denomination_sats || 0);
    if (denomination < SHIELDER_NOTE_MIN_SATS) {
      feeRemainder += denomination;
    } else {
      filtered.push(note);
    }
  }
  return {
    ...receipt,
    notes: filtered,
    remainder_sats: feeRemainder
  };
}

function encodePart(part) {
  const bytes = new TextEncoder().encode(String(part));
  const out = new Uint8Array(8 + bytes.length);
  const view = new DataView(out.buffer);
  view.setBigUint64(0, BigInt(bytes.length), false);
  out.set(bytes, 8);
  return out;
}

function bytesToHex(bytes) {
  return [...bytes].map((byte) => byte.toString(16).padStart(2, "0")).join("");
}

function hexToBytes(hex) {
  const clean = hex.trim().replace(/^0x/i, "").replace(/\s+/g, "");
  if (!clean || clean.length % 2 !== 0 || !/^[0-9a-f]+$/i.test(clean)) {
    throw new Error("seed hex must contain an even number of hex characters");
  }
  const bytes = new Uint8Array(clean.length / 2);
  for (let index = 0; index < bytes.length; index += 1) {
    bytes[index] = parseInt(clean.slice(index * 2, index * 2 + 2), 16);
  }
  return bytes;
}

async function hashPartsBytes(parts) {
  const encoded = parts.map(encodePart);
  const total = encoded.reduce((sum, bytes) => sum + bytes.length, 0);
  const merged = new Uint8Array(total);
  let offset = 0;
  for (const bytes of encoded) {
    merged.set(bytes, offset);
    offset += bytes.length;
  }
  const digest = await crypto.subtle.digest("SHA-256", merged);
  return new Uint8Array(digest);
}

async function hashParts(parts) {
  return bytesToHex(await hashPartsBytes(parts));
}

async function sha256(bytes) {
  return new Uint8Array(await crypto.subtle.digest("SHA-256", bytes));
}

function ripemd160(bytes) {
  const r1 = [0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,7,4,13,1,10,6,15,3,12,0,9,5,2,14,11,8,3,10,14,4,9,15,8,1,2,7,0,6,13,11,5,12,1,9,11,10,0,8,12,4,13,3,7,15,14,5,6,2,4,0,5,9,7,12,2,10,14,1,3,8,11,6,15,13];
  const r2 = [5,14,7,0,9,2,11,4,13,6,15,8,1,10,3,12,6,11,3,7,0,13,5,10,14,15,8,12,4,9,1,2,15,5,1,3,7,14,6,9,11,8,12,2,10,0,4,13,8,6,4,1,3,11,15,0,5,12,2,13,9,7,10,14,12,15,10,4,1,5,8,7,6,2,13,14,0,3,9,11];
  const s1 = [11,14,15,12,5,8,7,9,11,13,14,15,6,7,9,8,7,6,8,13,11,9,7,15,7,12,15,9,11,7,13,12,11,13,6,7,14,9,13,15,14,8,13,6,5,12,7,5,11,12,14,15,14,15,9,8,9,14,5,6,8,6,5,12,9,15,5,11,6,8,13,12,5,12,13,14,11,8,5,6];
  const s2 = [8,9,9,11,13,15,15,5,7,7,8,11,14,14,12,6,9,13,15,7,12,8,9,11,7,7,12,7,6,15,13,11,9,7,15,11,8,6,6,14,12,13,5,14,13,13,7,5,15,5,8,11,14,14,6,14,6,9,12,9,12,5,15,8,8,5,12,9,12,5,14,6,8,13,6,5,15,13,11,11];
  const rotl = (x, n) => ((x << n) | (x >>> (32 - n))) >>> 0;
  const f = (j, x, y, z) => j < 16 ? (x ^ y ^ z) : j < 32 ? ((x & y) | (~x & z)) : j < 48 ? ((x | ~y) ^ z) : j < 64 ? ((x & z) | (y & ~z)) : (x ^ (y | ~z));
  const k1 = (j) => j < 16 ? 0x00000000 : j < 32 ? 0x5a827999 : j < 48 ? 0x6ed9eba1 : j < 64 ? 0x8f1bbcdc : 0xa953fd4e;
  const k2 = (j) => j < 16 ? 0x50a28be6 : j < 32 ? 0x5c4dd124 : j < 48 ? 0x6d703ef3 : j < 64 ? 0x7a6d76e9 : 0x00000000;
  const msg = new Uint8Array((((bytes.length + 8) >>> 6) + 1) << 6);
  msg.set(bytes);
  msg[bytes.length] = 0x80;
  const bitLen = bytes.length * 8;
  for (let i = 0; i < 8; i += 1) msg[msg.length - 8 + i] = Math.floor(bitLen / 2 ** (8 * i)) & 0xff;
  let h0 = 0x67452301, h1 = 0xefcdab89, h2 = 0x98badcfe, h3 = 0x10325476, h4 = 0xc3d2e1f0;
  for (let offset = 0; offset < msg.length; offset += 64) {
    const x = new Array(16);
    for (let i = 0; i < 16; i += 1) x[i] = msg[offset + 4 * i] | (msg[offset + 4 * i + 1] << 8) | (msg[offset + 4 * i + 2] << 16) | (msg[offset + 4 * i + 3] << 24);
    let al = h0, bl = h1, cl = h2, dl = h3, el = h4;
    let ar = h0, br = h1, cr = h2, dr = h3, er = h4;
    for (let j = 0; j < 80; j += 1) {
      let t = (rotl((al + f(j, bl, cl, dl) + x[r1[j]] + k1(j)) >>> 0, s1[j]) + el) >>> 0;
      al = el; el = dl; dl = rotl(cl, 10); cl = bl; bl = t;
      t = (rotl((ar + f(79 - j, br, cr, dr) + x[r2[j]] + k2(j)) >>> 0, s2[j]) + er) >>> 0;
      ar = er; er = dr; dr = rotl(cr, 10); cr = br; br = t;
    }
    const t = (h1 + cl + dr) >>> 0;
    h1 = (h2 + dl + er) >>> 0;
    h2 = (h3 + el + ar) >>> 0;
    h3 = (h4 + al + br) >>> 0;
    h4 = (h0 + bl + cr) >>> 0;
    h0 = t;
  }
  const out = new Uint8Array(20);
  [h0, h1, h2, h3, h4].forEach((word, i) => {
    out[i * 4] = word & 0xff;
    out[i * 4 + 1] = (word >>> 8) & 0xff;
    out[i * 4 + 2] = (word >>> 16) & 0xff;
    out[i * 4 + 3] = (word >>> 24) & 0xff;
  });
  return out;
}

function bech32Encode(hrp, data) {
  const alphabet = "qpzry9x8gf2tvdw0s3jn54khce6mua7l";
  const polymod = (values) => {
    const gen = [0x3b6a57b2,0x26508e6d,0x1ea119fa,0x3d4233dd,0x2a1462b3];
    let chk = 1;
    for (const value of values) {
      const top = chk >>> 25;
      chk = ((chk & 0x1ffffff) << 5) ^ value;
      for (let i = 0; i < 5; i += 1) if ((top >>> i) & 1) chk ^= gen[i];
    }
    return chk;
  };
  const hrpValues = [...hrp].map((ch) => ch.charCodeAt(0) >>> 5).concat([0], [...hrp].map((ch) => ch.charCodeAt(0) & 31));
  const checksumBase = hrpValues.concat(data, [0,0,0,0,0,0]);
  const mod = polymod(checksumBase) ^ 1;
  const checksum = [];
  for (let i = 0; i < 6; i += 1) checksum.push((mod >>> (5 * (5 - i))) & 31);
  return `${hrp}1${data.concat(checksum).map((v) => alphabet[v]).join("")}`;
}

function convertBits(bytes, from, to, pad = true) {
  let acc = 0, bits = 0;
  const maxv = (1 << to) - 1;
  const out = [];
  for (const value of bytes) {
    acc = (acc << from) | value;
    bits += from;
    while (bits >= to) {
      bits -= to;
      out.push((acc >>> bits) & maxv);
    }
  }
  if (pad && bits > 0) out.push((acc << (to - bits)) & maxv);
  return out;
}

async function ownerAddressFromCompressedPubkey(pubkeyHex) {
  const hash = ripemd160(await sha256(hexToBytes(pubkeyHex)));
  return bech32Encode("tthor", convertBits(hash, 8, 5));
}

async function currentPowDifficultyBits() {
  const config = await api("/thornado/config").catch(() => null);
  const list = Array.isArray(config?.configs) ? config.configs : [];
  const current = list.find((item) => item.key === "Deposit_PowDifficultyCurrent");
  const minimum = list.find((item) => item.key === "Deposit_PowDifficultyMin");
  const bits = Number(
    current?.value
      || minimum?.value
      || config?.DEPOSIT_POWDIFFICULTYCURRENT?.value
      || config?.DEPOSIT_POWDIFFICULTYMIN?.value
      || config?.Deposit?.PowDifficultyCurrent?.value
      || POW_DIFFICULTY_BITS
  );
  return Number.isFinite(bits) && bits > 0 ? bits : POW_DIFFICULTY_BITS;
}

function bytesToBits(bytes) {
  return [...bytes].map((byte) => byte.toString(2).padStart(8, "0")).join("");
}

async function generateMnemonic12() {
  const entropy = new Uint8Array(16);
  crypto.getRandomValues(entropy);
  const checksum = bytesToBits(await sha256(entropy)).slice(0, 4);
  const bits = `${bytesToBits(entropy)}${checksum}`;
  const words = [];
  for (let offset = 0; offset < bits.length; offset += 11) {
    words.push(BIP39_WORDS[parseInt(bits.slice(offset, offset + 11), 2)]);
  }
  return words.join(" ");
}

function hasLeadingZeroBits(bytes, bits) {
  const fullZeroBytes = Math.floor(bits / 8);
  const remainingBits = bits % 8;
  for (let index = 0; index < fullZeroBytes; index += 1) {
    if (bytes[index] !== 0) {
      return false;
    }
  }
  if (remainingBits === 0) {
    return true;
  }
  const byte = bytes[fullZeroBytes];
  const mask = 0xff << (8 - remainingBits);
  return (byte & mask) === 0;
}

async function mineDepositPow(_label, owner, difficultyBits) {
  const uniqueLabel = bytesToHex(crypto.getRandomValues(new Uint8Array(16)));
  const startedAt = performance.now();
  const bits = Number(difficultyBits || POW_DIFFICULTY_BITS);
  if (window.Worker) {
    const workerSource = `
      const encoder = new TextEncoder();
      function bytesToHex(bytes) {
        return [...bytes].map((byte) => byte.toString(16).padStart(2, "0")).join("");
      }
      function hasLeadingZeroBits(bytes, bits) {
        const fullZeroBytes = Math.floor(bits / 8);
        const remainingBits = bits % 8;
        for (let index = 0; index < fullZeroBytes; index += 1) {
          if (bytes[index] !== 0) return false;
        }
        if (remainingBits === 0) return true;
        const byte = bytes[fullZeroBytes];
        const mask = 0xff << (8 - remainingBits);
        return (byte & mask) === 0;
      }
      async function sha256(bytes) {
        return new Uint8Array(await crypto.subtle.digest("SHA-256", bytes));
      }
      self.onmessage = async (event) => {
        const { owner, uniqueLabel, bits, minMs } = event.data;
        const startedAt = performance.now();
        try {
          for (let nonce = 0; nonce < Number.MAX_SAFE_INTEGER; nonce += 1) {
            const token = uniqueLabel + ":" + nonce;
            const digest = await sha256(encoder.encode(owner + ":" + token));
            const elapsedMs = performance.now() - startedAt;
            if (hasLeadingZeroBits(digest, bits) && elapsedMs >= minMs) {
              self.postMessage({
                ok: true,
                token,
                nonce,
                digest: bytesToHex(digest),
                difficulty_bits: bits,
                elapsed_ms: Math.round(elapsedMs)
              });
              return;
            }
            if (nonce > 0 && nonce % 5000 === 0) {
              self.postMessage({ progress: true, nonce, elapsed_ms: Math.round(elapsedMs) });
            }
          }
          self.postMessage({ ok: false, error: "unable to mine deposit proof of work" });
        } catch (error) {
          self.postMessage({ ok: false, error: error?.message || String(error) });
        }
      };
    `;
    const url = URL.createObjectURL(new Blob([workerSource], { type: "text/javascript" }));
    const worker = new Worker(url);
    return new Promise((resolve, reject) => {
      worker.onmessage = (event) => {
        if (event.data?.progress) return;
        worker.terminate();
        URL.revokeObjectURL(url);
        if (event.data?.ok) resolve(event.data);
        else reject(new Error(event.data?.error || "unable to mine deposit proof of work"));
      };
      worker.onerror = (event) => {
        worker.terminate();
        URL.revokeObjectURL(url);
        reject(new Error(event.message || "deposit proof worker failed"));
      };
      worker.postMessage({ owner, uniqueLabel, bits, minMs: MIN_POW_MS });
    });
  }
  for (let nonce = 0; nonce < Number.MAX_SAFE_INTEGER; nonce += 1) {
    const token = `${uniqueLabel}:${nonce}`;
    const digest = await sha256(new TextEncoder().encode(`${owner}:${token}`));
    const elapsedMs = performance.now() - startedAt;
    if (hasLeadingZeroBits(digest, bits) && elapsedMs >= MIN_POW_MS) {
      return {
        token,
        nonce,
        digest: bytesToHex(digest),
        difficulty_bits: bits,
        elapsed_ms: Math.round(elapsedMs)
      };
    }
    if (nonce > 0 && nonce % 2000 === 0) {
      await new Promise((resolve) => setTimeout(resolve, 0));
    }
  }
  throw new Error("unable to mine deposit proof of work");
}

function renderPowVisual() {
  if (!state.powVisual) {
    return;
  }
  const elapsed = performance.now() - state.powVisual.startedAt;
  const clamped = Math.max(0, elapsed);
  const linear = clamped / POW_VISUAL_REFERENCE_MS;
  const eased = 1 - Math.exp(-4.6 * linear);
  const angle = Math.min(356, eased * 360);
  $("powClock").style.setProperty("--pow-angle", `${angle}deg`);
  $("powCountdown").textContent = `${Math.max(1, Math.ceil(clamped / 1000))}s`;
}

function startPowVisual() {
  stopPowVisual(false);
  state.powVisual = {
    startedAt: performance.now(),
    timer: setInterval(renderPowVisual, 250)
  };
  $("powClock").style.setProperty("--pow-angle", "0deg");
  $("powCountdown").textContent = "1s";
  $("powStatus").hidden = false;
  renderPowVisual();
}

function stopPowVisual(hide = true) {
  if (state.powVisual?.timer) {
    clearInterval(state.powVisual.timer);
  }
  state.powVisual = null;
  if (hide) {
    $("powStatus").hidden = true;
  }
}

const SECRET_STORE_NAME = "thornado-secret-v1";
const SECRET_KEY_ID = "wallet-root";
const APP_STARTED_KEY = "thornado-app-started-v1";

function markAppStarted(persist = true) {
  state.appStarted = true;
  if (persist) {
    localStorage.setItem(APP_STARTED_KEY, "1");
  }
  renderIntroGate();
}

function renderIntroGate() {
  const intro = $("introGate");
  const stage = $("getAddressStage");
  const panel = $("userPanel");
  if (!intro || !stage) {
    return;
  }
  const isUserTab = state.currentTab === "user";
  intro.hidden = false;
  $("startFlow").hidden = !isUserTab || state.appStarted;
  if (panel && isUserTab) {
    panel.hidden = !state.appStarted;
  }
  stage.hidden = isUserTab && !state.appStarted;
}

function renderMoreNav() {
  const topbar = $("topbar");
  const button = $("moreToggle");
  const intro = $("introGate");
  if (!topbar || !button) {
    return;
  }
  topbar.classList.toggle("open", state.moreOpen);
  topbar.classList.toggle("settled", state.moreOpen && state.moreSettled);
  topbar.classList.toggle("closing", state.quoteWriting && !state.moreOpen);
  if (intro) {
    intro.classList.remove("more-open");
    intro.classList.toggle("quote-writing", state.quoteWriting && !state.moreOpen);
  }
  button.setAttribute("aria-expanded", state.moreOpen ? "true" : "false");
  button.textContent = state.moreOpen ? "Hide" : "Menu";
}

function toggleMoreNav() {
  const opening = !state.moreOpen;
  if (quoteWriteTimer) {
    clearTimeout(quoteWriteTimer);
    quoteWriteTimer = null;
  }
  if (moreRevealTimer) {
    clearTimeout(moreRevealTimer);
    moreRevealTimer = null;
  }
  state.moreOpen = opening;
  state.moreSettled = false;
  state.quoteWriting = !opening;
  if (opening) {
    moreRevealTimer = setTimeout(() => {
      state.moreSettled = true;
      moreRevealTimer = null;
      renderMoreNav();
    }, 520);
  }
  if (!opening) {
    closeStatusMenus();
    quoteWriteTimer = setTimeout(() => {
      state.quoteWriting = false;
      quoteWriteTimer = null;
      renderMoreNav();
    }, 1250);
  }
  renderMoreNav();
}

function base64FromBytes(bytes) {
  return btoa(String.fromCharCode(...bytes));
}

function bytesFromBase64(text) {
  return new Uint8Array([...atob(text)].map((ch) => ch.charCodeAt(0)));
}

function openSecretDb() {
  return new Promise((resolve, reject) => {
    if (!window.indexedDB) {
      reject(new Error("IndexedDB unavailable"));
      return;
    }
    const req = indexedDB.open(SECRET_STORE_NAME, 1);
    req.onupgradeneeded = () => req.result.createObjectStore("keys");
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

async function secretCryptoKey() {
  const db = await openSecretDb();
  const existing = await new Promise((resolve, reject) => {
    const req = db.transaction("keys", "readonly").objectStore("keys").get(SECRET_KEY_ID);
    req.onsuccess = () => resolve(req.result || null);
    req.onerror = () => reject(req.error);
  });
  if (existing) {
    db.close();
    return existing;
  }
  const key = await crypto.subtle.generateKey({ name: "AES-GCM", length: 256 }, false, ["encrypt", "decrypt"]);
  await new Promise((resolve, reject) => {
    const tx = db.transaction("keys", "readwrite");
    tx.objectStore("keys").put(key, SECRET_KEY_ID);
    tx.oncomplete = resolve;
    tx.onerror = () => reject(tx.error);
  });
  db.close();
  return key;
}

async function persistWalletSecret() {
  const mnemonic = $("walletRoot").value.trim();
  if (!mnemonic) return;
  const payload = JSON.stringify({
    mnemonic,
    passphrase: $("walletPassphrase").value || ""
  });
  if (!window.indexedDB || !window.crypto?.subtle) {
    localStorage.setItem(SECRET_STORE_NAME, JSON.stringify({
      version: 0,
      payload
    }));
    return;
  }
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const key = await secretCryptoKey();
  const ciphertext = new Uint8Array(await crypto.subtle.encrypt(
    { name: "AES-GCM", iv },
    key,
    new TextEncoder().encode(payload)
  ));
  localStorage.setItem(SECRET_STORE_NAME, JSON.stringify({
    version: 1,
    iv: base64FromBytes(iv),
    ciphertext: base64FromBytes(ciphertext)
  }));
}

async function restoreWalletSecret() {
  const stored = localStorage.getItem(SECRET_STORE_NAME);
  if (!stored) return false;
  try {
    const envelope = JSON.parse(stored);
    let payload;
    if (envelope.version === 0 && envelope.payload) {
      payload = JSON.parse(envelope.payload);
    } else {
      const key = await secretCryptoKey();
      const plain = await crypto.subtle.decrypt(
        { name: "AES-GCM", iv: bytesFromBase64(envelope.iv) },
        key,
        bytesFromBase64(envelope.ciphertext)
      );
      payload = JSON.parse(new TextDecoder().decode(plain));
    }
    const words = await validateMnemonic(payload.mnemonic, [12, 24]);
    $("walletRoot").value = words.join(" ");
    $("walletPassphrase").value = payload.passphrase || "";
    state.secretMode = "hidden";
    updateDashboard();
    return true;
  } catch (error) {
    localStorage.removeItem(SECRET_STORE_NAME);
    log("secret/restore", { error: error.message });
    return false;
  }
}

async function generateWalletRoot() {
  const mnemonic = await generateMnemonic12();
  $("walletRoot").value = mnemonic;
  state.secretMode = "hidden";
  await persistWalletSecret();
  const seedHex = await bip39SeedFromMnemonic(mnemonic, $("walletPassphrase").value);
  const pubkey = await clientPubkeyFromSeed(seedHex);
  $("walletRootStatus").hidden = true;
  $("walletRootStatus").textContent = `Generated 12-word BIP39 mnemonic in this browser. Root fingerprint ${await rootFingerprint(seedHex)}.`;
  $("clientPubkey").hidden = true;
  $("clientPubkey").textContent = `Client pubkey ${pubkey}`;
  updateNodeSecretStatus();
  updateDashboard();
}

async function bip39SeedFromMnemonic(mnemonic, passphrase) {
  const encoder = new TextEncoder();
  const key = await crypto.subtle.importKey(
    "raw",
    encoder.encode(mnemonic.normalize("NFKD")),
    "PBKDF2",
    false,
    ["deriveBits"]
  );
  const bits = await crypto.subtle.deriveBits(
    {
      name: "PBKDF2",
      hash: "SHA-512",
      iterations: 2048,
      salt: encoder.encode(`mnemonic${passphrase || ""}`.normalize("NFKD"))
    },
    key,
    512
  );
  return bytesToHex(new Uint8Array(bits));
}

async function walletRootSeedHex() {
  const root = $("walletRoot").value.trim();
  if (!root) {
    throw new Error("generate or paste a BIP39 root before shielding");
  }
  const words = await validateMnemonic(root, [12, 15, 18, 21, 24]);
  return bip39SeedFromMnemonic(words.join(" "), $("walletPassphrase").value);
}

async function validateMnemonic(mnemonic, allowedWordCounts) {
  const words = mnemonic.normalize("NFKD").toLowerCase().split(/\s+/).filter(Boolean);
  if (!allowedWordCounts.includes(words.length)) {
    throw new Error(`mnemonic must have ${allowedWordCounts.join(" or ")} words`);
  }
  const indexes = [];
  for (const word of words) {
    const index = BIP39_WORDS.indexOf(word);
    if (index === -1) {
      throw new Error(`unknown BIP39 word: ${word}`);
    }
    indexes.push(index);
  }
  const bits = indexes.map((index) => index.toString(2).padStart(11, "0")).join("");
  const entropyBitLength = Math.floor(bits.length * 32 / 33);
  const checksumBitLength = bits.length - entropyBitLength;
  const entropyBits = bits.slice(0, entropyBitLength);
  const checksumBits = bits.slice(entropyBitLength);
  const entropy = new Uint8Array(entropyBitLength / 8);
  for (let offset = 0; offset < entropy.length; offset += 1) {
    entropy[offset] = parseInt(entropyBits.slice(offset * 8, offset * 8 + 8), 2);
  }
  const expectedChecksum = bytesToBits(await sha256(entropy)).slice(0, checksumBitLength);
  if (checksumBits !== expectedChecksum) {
    throw new Error("mnemonic checksum is invalid");
  }
  return words;
}

async function rootFingerprint(seedHex) {
  return bytesToHex((await sha256(hexToBytes(seedHex))).slice(0, 4));
}

async function clientPubkeyFromSeed(seedHex, depositIndex = 0, depositType = "user") {
  const wasm = await thornadoWasm();
  if (wasm.clientPubkeyForDepositTypeJson) {
    return wasm.clientPubkeyForDepositTypeJson(seedHex, normalizeDepositType(depositType), BigInt(depositIndex));
  }
  if (wasm.clientPubkeyForDepositJson) {
    return wasm.clientPubkeyForDepositJson(seedHex, BigInt(depositIndex));
  }
  return wasm.clientPubkeyFromSecretJson(seedHex);
}

async function refreshClientPubkey(depositIndex = state.activeBatchIndex || 0) {
  const seedHex = await walletRootSeedHex();
  const depositType = activeDepositPurpose();
  const pubkey = await clientPubkeyFromSeed(seedHex, depositIndex, depositType);
  $("clientPubkey").hidden = true;
  $("clientPubkey").textContent = `${depositType === "node" ? "Node purpose" : "User purpose"} pubkey ${pubkey}`;
  return pubkey;
}

function notePath(depositIndex, noteIndex, depositType = "user") {
  return `m/44'/60'/${depositPurposeIndex(depositType)}'/${depositIndex}/${noteIndex}`;
}

function noteBucketPath(depositIndex, depositType = "user") {
  return `m/44'/60'/${depositPurposeIndex(depositType)}'/${depositIndex}/n`;
}

async function deriveShieldReceipt(depositId, amountSats, seedHex, depositIndex = state.activeBatchIndex || 0) {
  const wasm = await thornadoWasm();
  const fingerprint = await rootFingerprint(seedHex);
  const depositType = normalizeDepositType(activeBatch()?.depositType || state.depositPurpose);
  const receipt = JSON.parse(wasm.deriveShieldReceiptForDepositTypeJson
    ? wasm.deriveShieldReceiptForDepositTypeJson(depositId, depositType, BigInt(depositIndex), BigInt(amountSats), seedHex)
    : wasm.deriveShieldReceiptForDepositJson
      ? wasm.deriveShieldReceiptForDepositJson(depositId, BigInt(depositIndex), BigInt(amountSats), seedHex)
      : wasm.deriveShieldReceiptJson(depositId, BigInt(amountSats), seedHex));
  const filteredReceipt = applySpendableNoteFloor(receipt);
  filteredReceipt.notes = filteredReceipt.notes.map((note) => ({
    ...note,
    deposit_id: depositId,
    deposit_amount_sats: amountSats,
    deposit_remainder_sats: Number(filteredReceipt.remainder_sats || 0),
    deposit_index: depositIndex,
    deposit_type: depositType,
    derivation_path: notePath(depositIndex, note.index + 1, depositType),
    root_fingerprint: fingerprint
  }));
  return filteredReceipt;
}

async function deriveShieldReceiptForBatch(batch, seedHex, fingerprint) {
  const wasm = await thornadoWasm();
  const depositIndex = Number(batch.depositIndex || 0);
  const depositType = normalizeDepositType(batch.depositType);
  const depositId = batch.depositId || batch.inboundTxId || "";
  const amountSats = Number(batch.amountSats || 0);
  if (!depositId || !amountSats) {
    return null;
  }
  const receipt = JSON.parse(wasm.deriveShieldReceiptForDepositTypeJson
    ? wasm.deriveShieldReceiptForDepositTypeJson(depositId, depositType, BigInt(depositIndex), BigInt(amountSats), seedHex)
    : wasm.deriveShieldReceiptForDepositJson
      ? wasm.deriveShieldReceiptForDepositJson(depositId, BigInt(depositIndex), BigInt(amountSats), seedHex)
      : wasm.deriveShieldReceiptJson(depositId, BigInt(amountSats), seedHex));
  const filteredReceipt = applySpendableNoteFloor(receipt);
  filteredReceipt.notes = filteredReceipt.notes.map((note) => ({
    ...note,
    deposit_id: depositId,
    deposit_amount_sats: amountSats,
    deposit_remainder_sats: Number(filteredReceipt.remainder_sats || 0),
    deposit_index: depositIndex,
    deposit_type: depositType,
    derivation_path: notePath(depositIndex, note.index + 1, depositType),
    root_fingerprint: fingerprint,
    nullifier_hash: wasm.nullifierHashJson ? wasm.nullifierHashJson(note.nullifier) : note.nullifier_hash
  }));
  return filteredReceipt;
}

async function recoverKnownDepositReceipts(seedHex, sync, fingerprint) {
  const chainCommitments = new Set((sync.notes || []).map((item) => String(item.commitment || "").toLowerCase()));
  const spentNullifiers = new Map((sync.nullifiers || []).map((item) => [
    String(item.nullifier_hash || ""),
    item.withdrawal_id
  ]));
  const recovered = [];
  const withdrawn = [];
  for (const batch of state.batches) {
    if (!batchConfirmed(batch)) {
      continue;
    }
    const txs = mergeDepositTxs(batch.depositTxs);
    const receiptInputs = txs.length
      ? txs.map((tx) => ({
          ...batch,
          depositId: tx.txid,
          inboundTxId: tx.txid,
          amountSats: tx.amountSats
        }))
      : [batch];
    const matchedNotes = [];
    let remainderSats = 0;
    for (const input of receiptInputs) {
      const receipt = await deriveShieldReceiptForBatch(input, seedHex, fingerprint);
      const notes = (receipt?.notes || []).filter((note) => chainCommitments.has(String(note.commitment || "").toLowerCase()));
      if (notes.length) {
        matchedNotes.push(...notes);
        remainderSats += Number(receipt?.remainder_sats || 0);
      }
    }
    if (!matchedNotes.length) {
      continue;
    }
    for (const note of matchedNotes) {
      const spent = spentNullifiers.get(String(note.nullifier_hash || ""));
      note.spent = Boolean(spent);
      if (spent) {
        withdrawn.push({
          key: noteKey(note),
          txhash: spent,
          status: "spent",
          nullifierHash: note.nullifier_hash
        });
      }
    }
    recovered.push({
      ...batch,
      status: "committed",
      receipt: {
        notes: matchedNotes,
        remainder_sats: remainderSats
      },
      shieldedAt: batch.shieldedAt || Date.now() - DEMO_MATURITY_MS,
      maturesAt: batch.maturesAt || Date.now() - 1
    });
  }
  return { recovered, withdrawn };
}

async function shieldAuthorization(seedHex, depositId, amountSats, noteCommitments, depositIndex = state.activeBatchIndex || 0) {
  const wasm = await thornadoWasm();
  const depositType = normalizeDepositType(activeBatch()?.depositType || state.depositPurpose);
  return JSON.parse(wasm.shieldAuthorizationForDepositTypeJson
    ? wasm.shieldAuthorizationForDepositTypeJson(
        seedHex,
        depositType,
        BigInt(depositIndex),
        depositId,
        BigInt(amountSats),
        JSON.stringify(noteCommitments)
      )
    : wasm.shieldAuthorizationForDepositJson
      ? wasm.shieldAuthorizationForDepositJson(
        seedHex,
        BigInt(depositIndex),
        depositId,
        BigInt(amountSats),
        JSON.stringify(noteCommitments)
      )
      : wasm.shieldAuthorizationJson(
        seedHex,
        depositId,
        BigInt(amountSats),
        JSON.stringify(noteCommitments)
      ));
}

function commitments(receipt) {
  return receipt.notes.map((note) => ({
    denomination_sats: note.denomination_sats,
    commitment: note.commitment
  }));
}

function withdrawalProofContextKey(note, options = {}) {
  return JSON.stringify({
    note: noteKey(note),
    recipient: options.recipient || "",
    feeSats: options.feeSats ?? null,
    publicPatch: options.publicPatch || {}
  });
}

async function generateWithdrawalProof(noteIndex = state.selectedNote || 0, options = {}) {
  if (!state.receipt?.notes?.length) {
    throw new Error("shield into notes before withdrawing");
  }
  const note = state.receipt.notes[noteIndex];
  if (!note) {
    throw new Error("select a note to withdraw");
  }
  const recipient = options.recipient ?? $("recipient").value.trim();
  if (!recipient) {
    throw new Error("receive address is required");
  }
  const feeSats = options.feeSats ?? await quoteWithdrawalFee(note);
  const seedHex = await walletRootSeedHex();
  const leavesResponse = await shielderLeavesForDenomination(note.denomination_sats);
  updateNotePoolPosition(note, leavesResponse);
  renderNotes();
  updateDashboard();
  const wasm = await thornadoWasm();
  const [proof, generatedPublicInputs] = JSON.parse(wasm.zkWithdrawalFromReceiptJson(
    JSON.stringify(note),
    seedHex,
    JSON.stringify(leavesResponse.leaves),
    recipient,
    BigInt(feeSats)
  ));
  const publicInputs = {
    ...generatedPublicInputs,
    ...(options.publicPatch || {})
  };
  if (!proof?.tornado?.groth16) {
    if (!wasm.withdrawalWitnessFromReceiptJson) {
      throw new Error("Browser proof engine unavailable. Rebuild the web client with the Groth16 witness export before withdrawing.");
    }
    setMessage("Generating privacy proof in this browser...", "");
    const witness = JSON.parse(wasm.withdrawalWitnessFromReceiptJson(
      JSON.stringify(note),
      JSON.stringify(leavesResponse.leaves),
      recipient,
      BigInt(feeSats)
    ));
    const verifyBrowserProof = async () => {
      const groth16 = await proveWithdrawalInWorker(witness);
      const candidate = {
        ...proof,
        tornado: {
          ...(proof.tornado || {}),
          groth16
        }
      };
      wasm.verifyWithdrawalJson(JSON.stringify(candidate), JSON.stringify(publicInputs));
      return groth16;
    };
    proof.tornado = {
      ...(proof.tornado || {}),
      groth16: await proveWithdrawalWithFallback(witness, verifyBrowserProof)
    };
  }
  state.lastWithdrawalProof = proof;
  state.lastWithdrawalPublic = publicInputs;
  state.lastWithdrawalFeeSats = feeSats;
  state.lastWithdrawalNoteKey = noteKey(note);
  state.lastWithdrawalContextKey = withdrawalProofContextKey(note, options);
  return { proof, public: publicInputs, note };
}

async function proveWithdrawalWithFallback(witness, browserProofFn = null) {
  try {
    return browserProofFn ? await browserProofFn() : await proveWithdrawalInWorker(witness);
  } catch (error) {
    log("withdraw/proof/browser", { error: errorText(error) });
    setMessage("Browser proof worker failed; using local dev prover...", "warn", 8, 120000);
    return proveWithdrawalViaLocalServer(witness);
  }
}

async function proveWithdrawalViaLocalServer(witness) {
  const response = await fetch(new URL("/thornado/ui/prover/withdraw/prove", window.location.origin), {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(witness)
  });
  const text = await response.text();
  if (!response.ok) {
    throw new Error(text || "local dev prover failed");
  }
  return JSON.parse(text);
}

function proveWithdrawalInWorker(witness) {
  return new Promise((resolve, reject) => {
    if (!window.Worker) {
      reject(new Error("Browser worker support is required to generate the privacy proof."));
      return;
    }
    const proverVersion = new URLSearchParams(window.location.search).get("v") || "local";
    const proverUrl = new URL(`/thornado/ui/prover/prover.bundle.js?v=${encodeURIComponent(proverVersion)}`, window.location.origin).href;
    const circuitUrl = new URL(`/thornado/ui/prover/withdraw.json?v=${encodeURIComponent(proverVersion)}`, window.location.origin).href;
    const provingKeyUrl = new URL(`/thornado/ui/prover/withdraw_proving_key.bin?v=${encodeURIComponent(proverVersion)}`, window.location.origin).href;
    const workerSource = `
      self.window = self;
      self.navigator = self.navigator || { hardwareConcurrency: 1 };
      importScripts(${JSON.stringify(proverUrl)});
      let assetsPromise;
      async function assets() {
        if (!assetsPromise) {
          assetsPromise = Promise.all([
            fetch(${JSON.stringify(circuitUrl)}).then((response) => {
              if (!response.ok) throw new Error("withdraw circuit unavailable");
              return response.json();
            }),
            fetch(${JSON.stringify(provingKeyUrl)}).then((response) => {
              if (!response.ok) throw new Error("withdraw proving key unavailable");
              return response.arrayBuffer();
            })
          ]);
        }
        return assetsPromise;
      }
      self.onmessage = async (event) => {
        try {
          const [circuit, provingKey] = await assets();
          const groth16 = await self.ThornadoBrowserProver.proveWithdrawWithJson(event.data, circuit, provingKey);
          self.postMessage({ groth16 });
        } catch (error) {
          self.postMessage({ error: error.stack || error.message || String(error) });
        }
      };
    `;
    const url = URL.createObjectURL(new Blob([workerSource], { type: "text/javascript" }));
    const worker = new Worker(url);
    worker.onmessage = (event) => {
      worker.terminate();
      URL.revokeObjectURL(url);
      if (event.data?.error) {
        reject(new Error(event.data.error));
      } else {
        resolve(event.data.groth16);
      }
    };
    worker.onerror = (event) => {
      worker.terminate();
      URL.revokeObjectURL(url);
      reject(new Error(event.message || "privacy proof worker failed"));
    };
    worker.postMessage(witness);
  });
}

async function waitForDepositSession(owner, timeoutMs = 30000) {
  const started = Date.now();
  let lastError = null;
  while (Date.now() - started < timeoutMs) {
    try {
      const session = await api(`/thornado/deposit/session/${owner}`);
      if (session?.deposit_address) {
        return session;
      }
    } catch (error) {
      lastError = error;
    }
    await new Promise((resolve) => setTimeout(resolve, 1200));
  }
  throw lastError || new Error("deposit session was not committed");
}

async function waitForMatchedDepositSession(owner, timeoutMs = 60000) {
  const started = Date.now();
  let lastSession = null;
  let lastError = null;
  while (Date.now() - started < timeoutMs) {
    try {
      const session = await api(`/thornado/deposit/session/${owner}`);
      lastSession = session;
      if (session?.deposit_id) {
        return session;
      }
    } catch (error) {
      lastError = error;
    }
    await new Promise((resolve) => setTimeout(resolve, 1500));
  }
  if (lastSession) {
    throw new Error(`deposit not matched yet: ${lastSession.status || "unknown"}`);
  }
  throw lastError || new Error("deposit was not matched by the node");
}

function readNumber(...values) {
  for (const value of values) {
    const number = Number(value);
    if (Number.isFinite(number)) {
      return number;
    }
  }
  return null;
}

function confirmationProgressFromUi() {
  const text = String($("confirmations").textContent || "");
  const match = text.match(/(\d+)\s*\/\s*(\d+)/);
  const required = Number(match?.[2] || minConfirmations()) || minConfirmations();
  const current = Math.min(required, Number(match?.[1] || 0) || 0);
  return { current, required, seen: current > 0 || text.includes("seen") };
}

function setConfirmationProgress(current, required = minConfirmations(), seen = false) {
  const normalizedRequired = Math.max(1, Number(required || minConfirmations()) || minConfirmations());
  const normalizedCurrent = Math.max(0, Math.min(normalizedRequired, Number(current || 0) || 0));
  $("confirmations").textContent = `${normalizedCurrent} / ${normalizedRequired}${seen ? " seen" : ""}`;
  return { current: normalizedCurrent, required: normalizedRequired, seen };
}

function confirmationProgressLabel(progress = {}, suffix = "") {
  const required = Math.max(1, Number(progress.required || minConfirmations()) || minConfirmations());
  const current = Math.max(0, Math.min(required, Number(progress.current || 0) || 0));
  const prefix = progress.seen && current === 0 ? "Mempool · " : "";
  return `${prefix}${current} / ${required}${suffix}`;
}

function depositConfirmationProgress(session = {}, deposit = {}, txStatus = null) {
  session = session || {};
  deposit = deposit || {};
  const required = readNumber(
    session.btc_confirmations_required,
    session.BTCConfirmationsRequired,
    deposit.btc_confirmations_required,
    deposit.BTCConfirmationsRequired,
    minConfirmations()
  ) || minConfirmations();
  const txStages = txStatus?.stages || {};
  const observedStage = txStages.inbound_observed || {};
  const countedStage = txStages.inbound_confirmation_counted || {};
  const observedHeight = readNumber(
    session.btc_observed_height,
    session.BTCObservedHeight,
    deposit.btc_observed_height,
    deposit.BTCObservedHeight,
    countedStage.external_observed_height
  );
  const targetHeight = readNumber(countedStage.external_confirmation_delay_height);
  let current = readNumber(
    session.btc_confirmations,
    session.BTCConfirmations,
    deposit.btc_confirmations,
    deposit.BTCConfirmations,
    observedStage.final_count
  );
  const hasExplicitProgress = current !== null;
  if (current === null) {
    current = 0;
  }
  if (observedHeight !== null && targetHeight !== null && targetHeight >= observedHeight) {
    current = Math.max(current, required - Math.max(0, targetHeight - observedHeight));
  }
  const status = String(deposit.status || session.status || "");
  const hasDepositRecord = Boolean(deposit && Object.keys(deposit).length);
  const observedTxId = inboundTxId(session, deposit) || txStatus?.observed_tx?.tx?.id || txStatus?.txs?.[0]?.tx?.id || "";
  const seen = Boolean(
    observedHeight
      || readNumber(observedStage.pre_confirmation_count, observedStage.final_count)
      || status && status !== "address_issued"
      || session.deposit_id
      || deposit.deposit_id
      || observedTxId
  );
  const final = status === "deposit_matched"
    || status === "committed"
    || countedStage.completed === true
    || (!hasExplicitProgress && hasDepositRecord && !observedHeight);
  return {
    current: final ? required : Math.min(required, Math.max(0, current)),
    required,
    seen
  };
}

function inboundTxId(session = {}, deposit = {}) {
  return String(
    session?.inbound_tx_id
      || session?.InboundTxID
      || deposit?.inbound_tx_id
      || deposit?.InboundTxID
      || ""
  ).trim();
}

function txAmountSats(txStatus = null) {
  const coins = txStatus?.observed_tx?.tx?.coins
    || txStatus?.txs?.[0]?.tx?.coins
    || [];
  const coin = coins.find((item) => String(item.asset || "").toUpperCase() === "BTC.BTC") || coins[0];
  const amount = Number(coin?.amount || 0);
  return Number.isFinite(amount) ? amount : 0;
}

async function buildDepositTxScanCache() {
  const ids = new Set();
  const txout = await api("/thornado/txout").catch(() => null);
  for (const batch of txout?.txouts || []) {
    for (const item of batch.tx_array || []) {
      if (String(item.tx_type || "").toLowerCase() === "sweep" && item.in_hash) {
        ids.add(String(item.in_hash).toUpperCase());
      }
    }
  }
  return {
    ids,
    txById: new Map(),
    txsByAddress: new Map(),
    depositById: new Map()
  };
}

async function readDepositTxRecord(id, cache = null) {
  const key = String(id || "").toUpperCase();
  if (!key) {
    return null;
  }
  if (cache?.txById?.has(key)) {
    return cache.txById.get(key);
  }
  const txStatus = await api(`/thornado/tx/${key}`).catch(() => null);
  const tx = txStatus?.observed_tx?.tx || txStatus?.txs?.[0]?.tx || {};
  if (!tx?.id && !tx?.to_address) {
    cache?.txById?.set(key, null);
    return null;
  }
  const deposit = cache?.depositById?.has(key)
    ? cache.depositById.get(key)
    : await api(`/thornado/deposit/${key}`).catch(() => null);
  cache?.depositById?.set(key, deposit);
  const progress = depositConfirmationProgress({}, deposit, txStatus);
  const record = {
    txid: String(tx.id || key).toUpperCase(),
    amountSats: txAmountSats(txStatus),
    progress,
    txStatus,
    deposit,
    status: deposit?.status || (progress.current >= progress.required ? "deposit_matched" : progress.seen ? "deposit_observed" : "address_issued")
  };
  cache?.txById?.set(key, record);
  const address = String(tx.to_address || "").trim();
  if (cache && address) {
    const rows = cache.txsByAddress.get(address) || [];
    if (!rows.some((row) => row.txid === record.txid)) {
      rows.push(record);
      cache.txsByAddress.set(address, rows);
    }
  }
  return record;
}

async function depositTxsForAddress(address, seedTxIds = [], scanCache = null) {
  const wanted = String(address || "").trim();
  if (!wanted) {
    return [];
  }
  const records = [...(scanCache?.txsByAddress?.get(wanted) || [])];
  const ids = new Set(seedTxIds.filter(Boolean).map((value) => String(value).toUpperCase()));
  for (const id of scanCache?.ids || []) {
    ids.add(id);
  }
  for (const id of ids) {
    if (records.some((record) => record.txid === id)) {
      continue;
    }
    const record = await readDepositTxRecord(id, scanCache);
    const tx = record?.txStatus?.observed_tx?.tx || record?.txStatus?.txs?.[0]?.tx || {};
    if (record && String(tx.to_address || "").trim() === wanted) {
      records.push(record);
    }
  }
  records.sort((a, b) => String(a.txid).localeCompare(String(b.txid)));
  return records;
}

function applyDepositTxAggregate(batch, depositTxs = []) {
  if (!batch || !depositTxs.length) {
    return batch;
  }
  const txs = mergeDepositTxs(depositTxs);
  const latest = txs[txs.length - 1];
  batch.depositTxs = txs;
  batch.inboundTxId = latest?.txid || batch.inboundTxId;
  batch.depositId = latest?.deposit?.deposit_id || latest?.txid || batch.depositId;
  batch.amountSats = txs.reduce((sum, tx) => sum + Number(tx.amountSats || 0), 0);
  batch.deposit = latest?.deposit || batch.deposit;
  batch.txStatus = latest?.txStatus || batch.txStatus;
  if (txs.length && txs.every((tx) => tx.progress?.current >= tx.progress?.required)) {
    batch.status = batch.status === "committed" ? batch.status : "deposit_matched";
  } else if (txs.length) {
    batch.status = batch.status === "committed" ? batch.status : "deposit_observed";
  }
  return batch;
}

function recoverNotesOffThread(seedHex, sync, depositIndexes, fingerprint) {
  if (!window.Worker) {
    throw new Error("Web Worker unavailable for note recovery");
  }
  const wasmVersion = new URLSearchParams(window.location.search).get("v") || "local";
  const workerSource = `
    const WASM_URL = new URL("/thornado/ui/wasm/thornado_web_wasm.js?v=${encodeURIComponent(wasmVersion)}", self.location.origin).href;
    const WASM_BINARY_URL = new URL("/thornado/ui/wasm/thornado_web_wasm_bg.wasm?v=${encodeURIComponent(wasmVersion)}", self.location.origin).href;
    function noteKey(note) {
      return note?.commitment || \`\${note?.deposit_index || 0}:\${note?.denomination_sats || 0}:\${note?.index || 0}\`;
    }
    self.onmessage = async (event) => {
      try {
        const { seedHex, sync, depositIndexes, fingerprint, maturityMs } = event.data;
        const wasm = await import(WASM_URL);
        await wasm.default(WASM_BINARY_URL);
        const knownDepositIndexes = new Set((depositIndexes || []).map((value) => Number(value || 0)));
        const candidates = JSON.parse(wasm.note_recovery_candidates_json(seedHex, BigInt(32), BigInt(128)))
          .filter((candidate) => knownDepositIndexes.has(Number(candidate.deposit_index || 0)));
        const spentNullifiers = new Map((sync.nullifiers || []).map((item) => [
          String(item.nullifier_hash || "").toUpperCase(),
          item.withdrawal_id
        ]));
        const depositPubkeys = new Map();
        const recoveredByBatch = new Map();
        const withdrawnNotes = [];
        const maturedAt = Date.now() - 1;
        for (const record of sync.notes || []) {
          let note = null;
          let key = 0;
          const seenRecoveryCandidates = new Set();
          for (const candidate of candidates) {
            const depositType = String(candidate.deposit_type || "user").toLowerCase() === "node" ? "node" : "user";
            const candidateKey = \`\${depositType}:\${candidate.deposit_index}:\${candidate.index}\`;
            if (seenRecoveryCandidates.has(candidateKey)) {
              continue;
            }
            seenRecoveryCandidates.add(candidateKey);
            const depositIndex = Number(candidate.deposit_index || 0);
            const depositPubkeyKey = \`\${depositType}:\${depositIndex}\`;
            let depositPubkey = depositPubkeys.get(depositPubkeyKey);
            if (!depositPubkey) {
              depositPubkey = wasm.client_pubkey_for_deposit_type_json
                ? wasm.client_pubkey_for_deposit_type_json(seedHex, depositType, BigInt(depositIndex))
                : wasm.client_pubkey_for_deposit_json(seedHex, BigInt(depositIndex));
              depositPubkeys.set(depositPubkeyKey, depositPubkey);
            }
            try {
              note = JSON.parse(wasm.recover_note_receipt_for_deposit_type_json
                ? wasm.recover_note_receipt_for_deposit_type_json(
                    seedHex,
                    depositType,
                    BigInt(candidate.deposit_index),
                    BigInt(candidate.index),
                    depositPubkey,
                    BigInt(record.denomination_sats),
                    record.commitment
                  )
                : wasm.recover_note_receipt_json(
                    seedHex,
                    BigInt(candidate.deposit_index),
                    BigInt(candidate.index),
                    depositPubkey,
                    BigInt(record.denomination_sats),
                    record.commitment
                  ));
              note.deposit_type = depositType;
              key = candidateKey.split(":").slice(0, 2).join(":");
              break;
            } catch (_) {
              note = null;
            }
          }
          if (!note) {
            continue;
          }
          note.root_fingerprint = fingerprint;
          note.nullifier_hash = wasm.nullifier_hash_json(note.nullifier);
          const nullifierHashKey = String(note.nullifier_hash || "").toUpperCase();
          note.spent = spentNullifiers.has(nullifierHashKey);
          const noteDepositIndex = Number(note.deposit_index || 0);
          const batch = recoveredByBatch.get(key) || {
            depositIndex: noteDepositIndex,
            depositType: note.deposit_type || "user",
            amountSats: 0,
            status: "committed",
            shieldedAt: maturedAt - maturityMs,
            maturesAt: maturedAt,
            receipt: { notes: [], remainder_sats: 0 }
          };
          batch.amountSats += Number(note.denomination_sats || 0);
          batch.receipt.notes.push(note);
          recoveredByBatch.set(key, batch);
          if (note.spent) {
            withdrawnNotes.push({
              key: noteKey(note),
              txhash: spentNullifiers.get(nullifierHashKey),
              status: "spent",
              nullifierHash: note.nullifier_hash
            });
          }
        }
        const recoveredBatches = [...recoveredByBatch.values()].map((batch) => ({
          ...batch,
          receipt: {
            ...batch.receipt,
            notes: batch.receipt.notes.sort((a, b) => Number(a.index || 0) - Number(b.index || 0))
          }
        }));
        self.postMessage({
          ok: true,
          recoveredBatches,
          withdrawnNotes,
          publicNoteCount: (sync.notes || []).length,
          nullifierCount: (sync.nullifiers || []).length
        });
      } catch (error) {
        self.postMessage({ ok: false, error: error?.message || String(error) });
      }
    };
  `;
  const url = URL.createObjectURL(new Blob([workerSource], { type: "text/javascript" }));
  const worker = new Worker(url, { type: "module" });
  return new Promise((resolve, reject) => {
    worker.onmessage = (event) => {
      worker.terminate();
      URL.revokeObjectURL(url);
      if (event.data?.ok) {
        resolve(event.data);
      } else {
        reject(new Error(event.data?.error || "note recovery worker failed"));
      }
    };
    worker.onerror = (event) => {
      worker.terminate();
      URL.revokeObjectURL(url);
      reject(new Error(event.message || "note recovery worker failed"));
    };
    worker.postMessage({ seedHex, sync, depositIndexes, fingerprint, maturityMs: DEMO_MATURITY_MS });
  });
}

async function discoverDepositBatches(options = {}) {
  const recoverNotes = options.recoverNotes !== false;
  const preserveSelection = options.preserveSelection === true;
  const selectedKeyBeforeScan = state.paneBatchKeys.deposit || state.activeBatchKey || "";
  const requestedPurpose = activeDepositPurpose();
  if (!$("walletRoot").value.trim()) {
    return [];
  }
  const seedHex = await walletRootSeedHex();
  const wasm = await thornadoWasm();
  if (!wasm.noteRecoveryCandidatesJson || !wasm.recoverNoteReceiptJson || !wasm.nullifierHashJson || !wasm.clientPubkeyForDepositJson) {
    setMessage("Secret connected. Rebuild WASM to enable local note recovery.", "warn");
    return state.batches;
  }
  const nextByType = { user: 0, node: 0 };
  const scanCache = await buildDepositTxScanCache();
  for (const scanType of ["user", "node"]) {
    for (let depositIndex = 0; depositIndex < 32; depositIndex += 1) {
      const depositPubkey = await clientPubkeyFromSeed(seedHex, depositIndex, scanType);
      const owner = await ownerAddressFromCompressedPubkey(depositPubkey);
      const session = await api(`/thornado/deposit/session/${owner}`).catch(() => null);
      if (!session?.deposit_address) {
        nextByType[scanType] = depositIndex;
        break;
      }
      let amountSats = Number(session.amount_sats || 0);
      let status = session.status || "address_issued";
      let depositType = scanType;
      let deposit = null;
      let txStatus = null;
      let observedTxId = inboundTxId(session);
      if (session.deposit_id) {
        deposit = await api(`/thornado/deposit/${session.deposit_id}`).catch(() => null);
        observedTxId = inboundTxId(session, deposit) || session.deposit_id;
      }
      if (observedTxId) {
        txStatus = await api(`/thornado/tx/${observedTxId}`).catch(() => null);
      }
      if (!observedTxId && session.inbound_tx_id) {
        observedTxId = session.inbound_tx_id;
      }
      if (deposit) {
        amountSats = Number(deposit?.amount_sats || amountSats || 0);
        status = deposit?.status || status;
      }
      if (!amountSats) {
        amountSats = txAmountSats(txStatus);
      }
      const depositTxs = await depositTxsForAddress(session.deposit_address, [observedTxId], scanCache);
      if (depositTxs.length) {
        amountSats = Number(depositTxs[depositTxs.length - 1].amountSats || amountSats || 0);
        observedTxId = depositTxs[depositTxs.length - 1].txid;
        txStatus = depositTxs[depositTxs.length - 1].txStatus || txStatus;
        status = depositTxs[depositTxs.length - 1].status || status;
      }
      const progress = depositConfirmationProgress(session, deposit, txStatus);
      if (!deposit?.status && progress.seen && progress.current < progress.required) {
        status = "deposit_observed";
      }
      const baseBatch = {
        depositIndex,
        owner,
        pubkey: depositPubkey,
        depositType,
        depositId: session.deposit_id || "",
        inboundTxId: observedTxId,
        depositAddress: session.deposit_address,
        amountSats,
        status,
        session,
        deposit,
        txStatus,
        settlement: deposit?.settlement || "",
        nodePubKey: session.node_pub_key || deposit?.node_pub_key || ""
      };
      upsertBatch(applyDepositTxAggregate(baseBatch, depositTxs));
      nextByType[scanType] = depositIndex + 1;
    }
  }
  state.nextDepositIndexByType = nextByType;
  state.nextDepositIndex = Number(nextByType[requestedPurpose] || 0);
  const preservedBatch = preserveSelection
    ? state.batches.find((batch) => batchKey(batch) === String(selectedKeyBeforeScan))
    : null;
  const openIssuedBatch = [...state.batches].reverse().find((batch) => normalizeDepositType(batch.depositType) === requestedPurpose && batchIssuedUnexpired(batch));
  const latestBatch = state.batches[state.batches.length - 1];
  if (preservedBatch) {
    state.paneBatchKeys.deposit = batchKey(preservedBatch);
    activateDepositBatch(preservedBatch);
  } else if (openIssuedBatch) {
    state.nextDepositIndex = Number(openIssuedBatch.depositIndex || 0);
    supersedeOlderIssuedBatches(openIssuedBatch.depositIndex);
    activateDepositBatch(openIssuedBatch);
    $("stageDeposit").dataset.expanded = "1";
    $("stageDepositTrack").dataset.expanded = "1";
  } else if (latestBatch) {
    activateDepositBatch(latestBatch);
  }
  let knownRecovery = { recovered: [], withdrawn: [] };
  let sync = { notes: [], nullifiers: [] };
  if (recoverNotes) {
    try {
      state.noteRecoveryPending = true;
      state.noteRecoveryStatus = "searching_commitments";
      state.noteRecoveryBatchKey = state.paneBatchKeys.deposit || state.activeBatchKey || "";
      state.noteRecoveryProgress = { percent: 0, loaded: 0, total: 0 };
      updateDashboard();
      sync = await shielderSync({
        force: true,
        onProgress: (progress) => {
          setNoteRecoveryProgress(progress);
        }
      });
      state.noteRecoveryStatus = "searching_nullifiers";
      state.noteRecoveryProgress = {
        phase: "local",
        percent: 0,
        loaded: 0,
        total: (sync.notes?.length || 0) + (sync.nullifiers?.length || 0),
        notes: sync.notes?.length || 0,
        nullifiers: sync.nullifiers?.length || 0
      };
      updateDashboard();
      const fingerprint = await rootFingerprint(seedHex);
      knownRecovery = await recoverKnownDepositReceipts(seedHex, sync, fingerprint);
      if (!knownRecovery.recovered.length) {
        const depositIndexes = state.batches.map((batch) => Number(batch.depositIndex || 0));
        state.noteRecoveryProgress = {
          ...state.noteRecoveryProgress,
          percent: 50
        };
        updateDashboard();
        const workerRecovery = await recoverNotesOffThread(seedHex, sync, depositIndexes, fingerprint);
        knownRecovery = {
          recovered: workerRecovery.recoveredBatches || [],
          withdrawn: workerRecovery.withdrawnNotes || []
        };
      }
      state.noteRecoveryProgress = {
        ...state.noteRecoveryProgress,
        percent: 100,
        done: true
      };
      for (const item of knownRecovery.withdrawn || []) {
        state.withdrawnNotes[item.key] = {
          txhash: item.txhash,
          withdrawalID: item.txhash,
          status: item.status,
          nullifierHash: item.nullifierHash
        };
      }
      for (const batch of knownRecovery.recovered || []) {
        const selectedKey = state.paneBatchKeys.deposit || state.activeBatchKey || "";
        const selected = state.batches.find((item) => batchKey(item) === String(selectedKey));
        const batchType = normalizeDepositType(batch.depositType);
        const existing = selected && normalizeDepositType(selected.depositType) === batchType && Number(selected.depositIndex || 0) === Number(batch.depositIndex || 0)
          ? selected
          : state.batches.find((item) => normalizeDepositType(item.depositType) === batchType && Number(item.depositIndex || 0) === Number(batch.depositIndex || 0));
        upsertBatch({
          ...(existing || {}),
          ...batch,
          batchId: existing ? batchKey(existing) : batch.batchId
        });
      }
      hydrateReceipt();
      openWithdrawForRecoveredNotes();
      const recoveryTotal = Number(state.noteRecoveryProgress?.total || 0);
      state.noteRecoveryStatus = "done";
      state.noteRecoveryProgress = {
        ...state.noteRecoveryProgress,
        loaded: recoveryTotal || Number(state.noteRecoveryProgress?.loaded || 0),
        total: recoveryTotal,
        percent: 100,
        done: true
      };
      updateDashboard();
      setTimeout(() => {
        if (state.noteRecoveryStatus === "done" && state.noteRecoveryProgress?.done) {
          state.noteRecoveryProgress = null;
          updateDashboard();
        }
      }, 12000);
      hydrateWithdrawnNotePayouts()
        .then(() => {
          renderNotes();
          updateDashboard();
        })
        .catch((error) => log("withdraw/hydrate", { error: errorText(error) }));
    } catch (error) {
      state.noteRecoveryStatus = "error";
      state.noteRecoveryProgress = null;
      throw error;
    } finally {
      state.noteRecoveryPending = false;
    }
  }
  renderDepositHistory();
  renderNotes();
  if (recoverNotes) {
    setMessage(`Synced ${sync.notes?.length || 0} public notes and ${sync.nullifiers?.length || 0} spent nullifiers. Recovered ${state.receipt?.notes?.length || 0} local notes.`, knownRecovery.recovered.length ? "ok" : "warn");
  } else if (hasRecoverableDepositBatch()) {
    queueDepositRecovery();
  }
  const e2eRecipient = new URLSearchParams(location.search).get("e2eWithdraw");
  if (recoverNotes && e2eRecipient && !state.e2eWithdrawStarted) {
    state.e2eWithdrawStarted = true;
    const params = new URLSearchParams(location.search);
    params.set("v", "e2e-withdraw-running");
    history.replaceState(null, "", `${location.pathname}?${params.toString()}`);
    try {
      const candidate = await firstWithdrawableE2eNote();
      if (!candidate) {
        throw new Error("no mature unspent note above the withdrawal fee for e2e withdrawal");
      }
      state.receipt = candidate.batch.receipt;
      state.activeBatchIndex = Number(candidate.batch.depositIndex || 0);
      state.selectedNote = candidate.index;
      $("recipient").value = e2eRecipient;
      log("e2e/withdraw-start", {
        batch: batchLabel(candidate.batch),
        note_index: candidate.note.note_index ?? candidate.index,
        amount: btcAmount(candidate.note.denomination_sats),
        fee: btcAmount(candidate.feeSats),
        recipient: e2eRecipient
      });
      await withdrawNote(state.selectedNote);
    } catch (error) {
      const message = `E2E withdrawal failed: ${errorText(error)}`;
      log("e2e/withdraw-error", { error: errorText(error) });
      setMessage(message, "error", 10, 300000);
      throw error;
    }
  }
  return state.batches;
}

async function waitForCommittedTx(txhash, timeoutMs = 30000) {
  if (!txhash) {
    return null;
  }
  const started = Date.now();
  let lastError = null;
  while (Date.now() - started < timeoutMs) {
    try {
      const payload = await api(`/cosmos/tx/v1beta1/txs/${txhash}`);
      const response = payload?.tx_response;
      if (response) {
        if (Number(response.code || 0) !== 0) {
          throw new Error(response.raw_log || `transaction ${txhash} failed`);
        }
        return response;
      }
    } catch (error) {
      lastError = error;
      if (!String(error.message || "").toLowerCase().includes("not found")) {
        throw error;
      }
    }
    await new Promise((resolve) => setTimeout(resolve, 1200));
  }
  throw lastError || new Error(`transaction ${txhash} was not committed`);
}

async function quoteWithdrawalFee(note) {
  const feeSats = withdrawalFeeForDenomination(note?.denomination_sats);
  if (!Number.isSafeInteger(feeSats) || feeSats < 0) {
    throw new Error("invalid withdrawal amount");
  }
  $("feeSats").value = String(feeSats);
  $("feePreview").textContent = btcAmount(feeSats);
  return feeSats;
}

async function validateGeneratedProof(noteIndex = state.selectedNote || 0, options = {}) {
  let proof = state.lastWithdrawalProof;
  let publicInputs = state.lastWithdrawalPublic;
  const requestedFee = Number($("feeSats").value || 0);
  const note = state.receipt?.notes?.[noteIndex];
  const hasExplicitFee = Object.prototype.hasOwnProperty.call(options, "feeSats");
  const contextKey = withdrawalProofContextKey(note, options);
  if (!proof || !publicInputs || (!hasExplicitFee && state.lastWithdrawalFeeSats !== requestedFee) || state.lastWithdrawalContextKey !== contextKey) {
    const generated = await generateWithdrawalProof(noteIndex, options);
    proof = generated.proof;
    publicInputs = generated.public;
  }
  const wasm = await thornadoWasm();
  wasm.verifyWithdrawalJson(JSON.stringify(proof), JSON.stringify(publicInputs));
  log("withdraw/proof/validate", {
    nullifier_hash: publicInputs.nullifier_hash,
    merkle_root: publicInputs.merkle_root,
    proof_bytes: proof.tornado?.groth16 ? JSON.stringify(proof.tornado.groth16).length : 0
  });
  setMessage("Withdrawal proof validates locally.", "ok");
  updateDashboard();
  return { proof, public: publicInputs };
}

function globalNoteIndex(note) {
  const notes = state.receipt?.notes || [];
  return notes.findIndex((item) => noteKey(item) === noteKey(note));
}

function noteCoversCurrentFee(note) {
  return Number(note?.denomination_sats || 0) > withdrawalFeeForDenomination(note?.denomination_sats);
}

async function firstWithdrawableNoteInBatch(batch) {
  if (!batch?.receipt?.notes?.length || !batchMature(batch)) {
    return null;
  }
  state.receipt = batch.receipt;
  state.activeBatchIndex = Number(batch.depositIndex || 0);
  for (let index = 0; index < batch.receipt.notes.length; index += 1) {
    const note = batch.receipt.notes[index];
    if (note.spent || state.withdrawnNotes[noteKey(note)]) {
      continue;
    }
    try {
      const feeSats = await quoteWithdrawalFee(note);
      if (Number(note.denomination_sats || 0) > feeSats) {
        return { batch, note, index, feeSats };
      }
    } catch (error) {
      log("e2e/fee-quote", { batch: batchLabel(batch), note_index: note.note_index ?? index, error: errorText(error) });
    }
  }
  return null;
}

async function firstWithdrawableE2eNote() {
  const orderedBatches = [...state.batches].sort((a, b) => Number(b.depositIndex || 0) - Number(a.depositIndex || 0));
  for (const batch of orderedBatches) {
    const candidate = await firstWithdrawableNoteInBatch(batch);
    if (candidate) {
      return candidate;
    }
  }
  return null;
}

function protocolRedeemOptions(policy, extra = {}) {
  return {
    recipient: "bond_escrow",
    feeSats: 0,
    publicPatch: {
      recipient_policy: policy,
      ...extra
    }
  };
}

function firstProtocolRedeemableNodeNote() {
  const active = activeNodeBatch();
  const batches = [
    ...(active ? [active] : []),
    ...state.batches
      .filter((batch) => normalizeDepositType(batch.depositType) === "node" && batch !== active)
      .sort((a, b) => Number(b.depositIndex || 0) - Number(a.depositIndex || 0))
  ];
  for (const batch of batches) {
    if (!batch?.receipt?.notes?.length || !batchMature(batch)) {
      continue;
    }
    state.receipt = batch.receipt;
    state.activeBatchIndex = Number(batch.depositIndex || 0);
    state.activeBatchKey = batchKey(batch);
    const index = batch.receipt.notes.findIndex((note) => !note.spent && !state.withdrawnNotes[noteKey(note)]);
    if (index >= 0) {
      state.selectedNote = index;
      return { batch, note: batch.receipt.notes[index], index };
    }
  }
  return null;
}

function renderShieldBatches() {
  const el = $("shieldBatchList");
  if (!el) {
    return;
  }
  el.textContent = "";
  const batches = visibleDepositBatches({ includeExpired: true, includeIssued: true });
  const selectedBatch = selectedFlowBatch(batches);
  if (!selectedBatch) {
    const row = document.createElement("div");
    row.className = "pane-empty";
    row.textContent = "Select a deposit first.";
    el.append(row);
    return;
  }
  const card = document.createElement("div");
  card.className = "batch-card";

  const progress = batchConfirmationProgress(selectedBatch);
  const notes = selectedBatch.receipt?.notes || [];
  const txRows = mergeDepositTxs(selectedBatch.depositTxs);
  const fallbackTxid = selectedBatch.inboundTxId || selectedBatch.depositId || txRows[0]?.txid || "";

  const appendDepositTxRows = () => {
    if (txRows.length) {
      for (const tx of txRows) {
        const header = document.createElement("div");
        header.className = "shield-summary-row";
        header.innerHTML = `<span>${txHashLink(tx.txid)}</span><strong>${btcAmount(Number(tx.amountSats || 0))}</strong>`;
        card.append(header);
      }
      return;
    }
    const amount = Number(selectedBatch.amountSats || 0);
    if (fallbackTxid || amount) {
      const header = document.createElement("div");
      header.className = "shield-summary-row";
      header.innerHTML = `<span>${fallbackTxid ? txHashLink(fallbackTxid) : "pending"}</span><strong>${btcAmount(amount)}</strong>`;
      card.append(header);
    }
  };

  if (noteRecoveryVisible()) {
    if (!notes.length) {
      appendDepositTxRows();
    }
    card.append(renderScanProgressRow(noteRecoveryLabel()));
  }

  if (notes.length) {
    const mature = batchMature(selectedBatch);
    const groups = new Map();
    for (const note of notes) {
      const txid = note.deposit_id || fallbackTxid || "pending";
      if (!groups.has(txid)) {
        groups.set(txid, {
          txid,
          amountSats: Number(note.deposit_amount_sats || 0),
          notes: []
        });
      }
      const group = groups.get(txid);
      group.notes.push(note);
      if (!group.amountSats) {
        group.amountSats += Number(note.denomination_sats || 0);
      }
    }
    for (const group of groups.values()) {
      const header = document.createElement("div");
      header.className = "shield-summary-row";
      header.innerHTML = `<span>${group.txid !== "pending" ? txHashLink(group.txid) : "pending"}</span><strong>${btcAmount(group.amountSats)}</strong>`;
      card.append(header);
      for (const note of group.notes) {
        const withdrawal = state.withdrawnNotes[noteKey(note)];
        const spent = note.spent || withdrawal?.status === "spent" || Boolean(withdrawal?.outHash);
        const row = document.createElement("div");
        row.className = "batch-note-row";
        row.innerHTML = `<span>${btcAmount(note.denomination_sats)}</span><strong>${spent ? "Withdrawn" : mature ? "Mature" : "Maturing"}</strong>`;
        card.append(row);
      }
    }
  } else if (noteRecoveryVisible()) {
    // Progress row rendered above the empty state.
  } else if (String(selectedBatch.status || "").toLowerCase() === "committed") {
    appendDepositTxRows();
    const row = document.createElement("div");
    row.className = "batch-note-row";
    row.innerHTML = "<span>Already shielded</span><strong>Searching notes...</strong>";
    card.append(row);
  } else if (batchFinalised(selectedBatch)) {
    appendDepositTxRows();
    const actionRow = document.createElement("div");
    actionRow.className = "batch-note-row";
    const button = document.createElement("button");
    button.type = "button";
    button.disabled = state.shieldPending;
    button.innerHTML = state.shieldPending && batchKey(selectedBatch) === String(state.activeBatchKey || "")
      ? '<span class="button-spinner" aria-hidden="true"></span>Shielding'
      : "Shield Deposit";
    button.addEventListener("click", () => run(() => shieldDeposit(batchKey(selectedBatch))));
    actionRow.innerHTML = "<span></span>";
    actionRow.append(button);
    card.append(actionRow);
  } else {
    const row = document.createElement("div");
    row.className = "batch-note-row";
    row.innerHTML = `<span>${progress.seen ? "Deposit finalising" : "Waiting for deposit"}</span><strong>${fallbackTxid ? txHashLink(fallbackTxid) : "pending"}</strong>`;
    card.append(row);
  }
  el.append(card);
}

function renderScanProgressRow(label) {
  const progress = state.noteRecoveryProgress || {};
  const percent = Math.max(0, Math.min(100, Number(progress.percent || 0)));
  const loaded = Number(progress.loaded || 0);
  const total = Number(progress.total || 0);
  const detail = total > 0
    ? `${Math.min(loaded, total)} / ${total}`
    : `${percent}%`;
  const row = document.createElement("div");
  row.className = "scan-progress-row";
  row.innerHTML = `
    <div class="scan-progress-copy">
      <span>${label}</span>
      <strong>${detail}</strong>
    </div>
    <div class="scan-progress-track" aria-hidden="true"><span style="width:${percent}%"></span></div>
  `;
  return row;
}

function renderNotes() {
  const el = $("notes");
  el.textContent = "";
  const selectedBatch = selectedFlowBatch();
  if (!selectedBatch) {
    const row = document.createElement("div");
    row.className = "pane-empty";
    row.textContent = "Select a deposit first.";
    el.append(row);
    return;
  }
  if (noteRecoveryVisible()) {
    el.append(renderScanProgressRow(noteRecoveryLabel()));
    if (!selectedBatch.receipt?.notes?.length) {
      return;
    }
  }
  if (!selectedBatch.receipt?.notes?.length) {
    const row = document.createElement("div");
    row.className = "pane-empty";
    row.textContent = batchConfirmed(selectedBatch) ? "No notes found for this deposit yet." : "Waiting for confirmed deposit.";
    el.append(row);
    return;
  }
  if (!batchMature(selectedBatch)) {
    const row = document.createElement("div");
    row.className = "pane-empty";
    row.textContent = `Maturing ${formatWaitClock(batchMaturityMs(selectedBatch))}`;
    el.append(row);
    return;
  }
  const card = document.createElement("div");
  card.className = "batch-card";
  const userNoteBuckets = {};
  for (const note of selectedBatch.receipt.notes) {
    const denomination = Number(note.denomination_sats || 0);
    if (denomination) {
      userNoteBuckets[denomination] = (userNoteBuckets[denomination] || 0) + 1;
    }
  }
  const publicBuckets = state.publicNoteBuckets || {};
  if (!Object.keys(publicBuckets).length) {
    refreshPublicNoteBuckets().catch((error) => log("pool/sync", { error: errorText(error) }));
  }
  for (const note of selectedBatch.receipt.notes) {
    const denomination = Number(note.denomination_sats || 0);
    const otherNotes = Math.max(0, Number(publicBuckets[denomination] || 0) - Number(userNoteBuckets[denomination] || 0));
    const index = globalNoteIndex(note);
    const key = noteKey(note);
    const withdrawal = state.withdrawnNotes[key];
    const isSpent = note.spent || withdrawal?.status === "spent";
    const isWithdrawing = state.withdrawingNote === key;
    const feeCovered = noteCoversCurrentFee(note);
    const row = document.createElement("div");
    row.className = "batch-note-row";
    row.dataset.testid = "withdraw-note-row";
    row.dataset.batchIndex = String(selectedBatch.depositIndex ?? "");
    row.dataset.noteIndex = String(note.note_index ?? index ?? "");
    row.dataset.noteAmount = String(note.denomination_sats ?? "");
    row.dataset.noteKey = key;
    const action = document.createElement("button");
    action.type = "button";
    action.dataset.testid = "withdraw-note";
    action.dataset.batchIndex = String(selectedBatch.depositIndex ?? "");
    action.dataset.noteIndex = String(note.note_index ?? index ?? "");
    action.dataset.noteAmount = String(note.denomination_sats ?? "");
    const isPending = isWithdrawing || (withdrawal && !withdrawal.outHash && withdrawal.status !== "spent");
    const isWithdrawn = isSpent || Boolean(withdrawal?.outHash);
    action.textContent = isWithdrawn ? "Withdrawn" : isPending ? "Pending" : feeCovered ? "Withdraw" : "Fee too high";
    action.disabled = isWithdrawn || isPending || !feeCovered;
    action.addEventListener("click", () => {
      state.receipt = selectedBatch.receipt;
      state.activeBatchIndex = Number(selectedBatch.depositIndex || 0);
      state.activeBatchKey = batchKey(selectedBatch);
      state.selectedNote = selectedBatch.receipt.notes.findIndex((item) => noteKey(item) === key);
      openWithdrawAddressModal(Math.max(0, state.selectedNote));
    });
    row.innerHTML = `
      <span>${btcAmount(note.denomination_sats)}</span>
      <span class="pool-inline-count">${otherNotes} other ${otherNotes === 1 ? "note" : "notes"}</span>
    `;
    row.append(action);
    if (withdrawal?.outHash || isPending) {
      const detail = document.createElement("div");
      detail.className = "row note-withdrawal";
      detail.innerHTML = `<span>Tx ID</span><strong>${withdrawal?.outHash ? txHashLink(withdrawal.outHash) : "Pending"}</strong>`;
      row.append(detail);
    }
    card.append(row);
  }
  el.append(card);
}

async function requestDeposit() {
  state.depositRequestPending = true;
  state.depositAutoConfirming = false;
  state.depositExpiresAt = null;
  state.depositExpiresAtHeight = null;
  state.addressPaneFocusKey = "__pending__";
  $("stageDeposit").dataset.expanded = "1";
  $("requestDeposit").disabled = true;
  $("depositResult").hidden = true;
  $("depositAddress").textContent = "";
  renderDepositQr("");
  if (!$("walletRoot").value.trim()) {
    await generateWalletRoot();
  }
  try {
    setMessage("Checking deposit addresses...", "");
    await discoverDepositBatches({ recoverNotes: false }).catch((error) => {
      const message = errorText(error);
      log("deposit/scan", { error: message });
      setMessage(`Deposit scan skipped: ${message}`, "warn", 1, 15000);
      return state.batches;
    });
    const reusableBatch = activeBatch();
    if (batchIssuedUnexpired(reusableBatch)) {
      activateDepositBatch(reusableBatch);
      state.addressPaneFocusKey = batchKey(reusableBatch);
      $("depositAddress").hidden = false;
      $("stageDeposit").dataset.expanded = "1";
      $("stageDepositTrack").dataset.expanded = "1";
      renderDepositHistory();
      setMessage("Using existing unexpired deposit address.", "ok", 1, 15000);
      autoConfirmDeposit();
      queueDepositRecovery();
      return;
    }
    startPowVisual();
    const depositType = activeDepositPurpose();
    const depositIndex = Number(state.nextDepositIndexByType[depositType] || state.nextDepositIndex || 0);
    state.activeBatchIndex = depositIndex;
    state.shieldStageOpenedForDeposit = false;
    setMessage(`Deriving the next ${depositType === "node" ? "node lifecycle" : "user"} deposit key...`, "");
    const userPubkey = await refreshClientPubkey(depositIndex);
    const owner = await ownerAddressFromCompressedPubkey(userPubkey);
    setMessage("Finding deposit proof...", "");
    const mined = await mineDepositPow("browser", owner, await currentPowDifficultyBits());
    $("powResult").hidden = true;
    $("powResult").textContent = `PoW ${mined.token}`;
    setMessage("Requesting deposit address from the node...", "");
    const payload = await api("/thornado/deposit", {
      method: "POST",
      body: {
        pow_token: mined.token,
        deposit_pubkey: userPubkey,
        pow_duration_ms: mined.elapsed_ms
      }
    });
    setMessage("Waiting for the address to commit on-chain...", "");
    const session = await waitForDepositSession(payload.owner);
    const churnWindow = await refreshChurnWindow();
    state.activeIntentId = userPubkey;
    $("intentId").value = state.activeIntentId;
    state.depositOwner = payload.owner;
    applyChurnWindow(churnWindow);
    applyDepositExpiry(session);
    const newBatch = {
      depositIndex,
      owner: payload.owner,
      pubkey: userPubkey,
      depositType,
      depositId: session.deposit_id || "",
      inboundTxId: inboundTxId(session),
      depositAddress: session.deposit_address,
      amountSats: Number(session.amount_sats || 0),
      status: session.status || "address_issued",
      session
    };
    upsertBatch(newBatch);
    supersedeOlderIssuedBatches(depositIndex);
    activateDepositBatch(newBatch);
    state.addressPaneFocusKey = batchKey(newBatch);
    state.depositDropdownOpen = true;
    state.waitStartedAt = null;
    state.waitMaturesAt = null;
    $("depositAddress").hidden = false;
    $("stageDeposit").dataset.expanded = "1";
    $("stageDepositTrack").dataset.expanded = "1";
    setConfirmationProgress(0, minConfirmations(), false);
    renderDepositHistory();
    log("deposit/request", { batch: batchLabel({ depositIndex }), user_pubkey: userPubkey, owner, pow: mined, response: payload, session });
    setMessage("Deposit address committed on-chain.", "ok");
    autoConfirmDeposit();
    queueDepositRecovery();
  } finally {
    state.depositRequestPending = false;
    stopPowVisual();
    $("requestDeposit").disabled = false;
    updateDashboard();
  }
}

function queueDepositRecovery() {
  if (state.noteRecoveryPending || state.noteRecoveryQueued || !hasRecoverableDepositBatch()) {
    return;
  }
  state.noteRecoveryQueued = true;
  state.noteRecoveryStatus = "searching_commitments";
  state.noteRecoveryBatchKey = state.paneBatchKeys.deposit || state.activeBatchKey || "";
  state.noteRecoveryProgress = { percent: 0, loaded: 0, total: 0 };
  updateDashboard();
  setTimeout(() => {
    state.noteRecoveryQueued = false;
    discoverDepositBatches({ recoverNotes: true, preserveSelection: true }).catch((error) => {
      const message = errorText(error);
      state.noteRecoveryPending = false;
      state.noteRecoveryQueued = false;
      state.noteRecoveryStatus = "error";
      state.noteRecoveryProgress = null;
      log("deposit/recovery", { error: message });
      setMessage(`Note recovery skipped: ${message}`, "warn", 9, 30000);
      updateDashboard();
    });
  }, 0);
}

async function confirmDeposit() {
  const expectedIntentId = state.activeIntentId || activeBatch()?.pubkey || activeBatch()?.depositId || "";
  const intentId = $("intentId").value.trim() || expectedIntentId;
  if (!expectedIntentId) {
    throw new Error("Request a deposit address before observing a deposit.");
  }
  if (intentId !== expectedIntentId) {
    throw new Error(`Active deposit intent is ${expectedIntentId}; request a fresh address before observing ${intentId}.`);
  }
  const amountSats = getAmountSats();
  if (!state.depositOwner) {
    throw new Error("request a deposit address before checking deposit status");
  }
  const trackingBatch = state.batches.find((batch) => batch.owner && batch.owner === state.depositOwner);
  const trackingDepositIndex = Number(trackingBatch?.depositIndex ?? state.activeBatchIndex ?? 0);
  const session = await api(`/thornado/deposit/session/${state.depositOwner}`);
  if (!session?.deposit_address) {
    throw new Error("deposit session was not found");
  }
  $("intentId").value = state.activeIntentId;
  const deposit = session.deposit_id
    ? await api(`/thornado/deposit/${session.deposit_id}`).catch(() => null)
    : null;
  let observedTxId = inboundTxId(session, deposit) || session.deposit_id || "";
  const txStatus = observedTxId
    ? await api(`/thornado/tx/${observedTxId}`).catch(() => null)
    : null;
  const depositTxs = await depositTxsForAddress(session.deposit_address, [observedTxId]);
  if (depositTxs.length) {
    observedTxId = depositTxs[depositTxs.length - 1].txid;
  }
  const aggregateTxStatus = depositTxs[depositTxs.length - 1]?.txStatus || txStatus;
  const progress = depositConfirmationProgress(session, deposit, aggregateTxStatus);
  const matchedAmount = depositTxs.length
    ? Number(depositTxs[depositTxs.length - 1].amountSats || 0)
    : Number(deposit?.amount_sats || session.amount_sats || txAmountSats(txStatus) || 0);
  if (Number.isFinite(matchedAmount) && matchedAmount > 0) {
    $("amountSats").value = String(matchedAmount);
  }
  const baseBatch = {
    depositIndex: trackingDepositIndex,
    owner: state.depositOwner,
    depositId: session.deposit_id,
    inboundTxId: observedTxId,
    depositAddress: session.deposit_address,
    amountSats: matchedAmount,
    status: deposit?.status || session.status || (progress.current >= progress.required ? "deposit_matched" : progress.seen ? "deposit_observed" : "address_issued"),
    session,
    deposit,
    txStatus: aggregateTxStatus
  };
  const aggregatedBatch = applyDepositTxAggregate(baseBatch, depositTxs);
  upsertBatch(aggregatedBatch);
  activateDepositBatch(aggregatedBatch);
  const batch = activeBatch();
  if (batch?.status === "committed" && batch.receipt?.notes?.length) {
    upsertBatch({
      depositIndex: trackingDepositIndex,
      shieldedAt: batch.shieldedAt || Date.now(),
      maturesAt: batch.maturesAt || Date.now() + DEMO_MATURITY_MS
    });
    await refreshReceiptPoolPositions();
    renderNotes();
  }
  applyDepositExpiry(session, deposit);
  log("deposit/status", { session, deposit, txStatus, progress });
  setConfirmationProgress(progress.current, progress.required, progress.seen);
  renderDepositHistory();
  if (progress.current >= progress.required) {
    const batch = activeBatch();
    if (batch?.status === "committed") {
      setMessage(`Deposit confirmed and shielded into ${batch.receipt?.notes?.length || 0} notes.`, "ok", 2, 15000);
    } else {
      setMessage("Deposit confirmed.", "ok", 2, 15000);
    }
    queueDepositRecovery();
  } else if (progress.seen) {
    setMessage(`Deposit seen. Waiting for confirmations: ${confirmationProgressLabel(progress)}.`, "warn", 1, 15000);
  } else {
    setMessage("Waiting for BTC deposit at this address.", "warn", 1, 15000);
  }
  updateDashboard();
}

async function autoConfirmDeposit() {
  if (state.depositAutoConfirming || !state.depositOwner) {
    return;
  }
  state.depositAutoConfirming = true;
  try {
    await confirmDeposit();
  } catch (error) {
    const message = errorText(error);
    const reconnecting = /bad gateway|failed to fetch|networkerror|load failed|connection|econnrefused|502|503|504/i.test(message);
    const friendly = reconnecting
      ? "Node reconnecting. Deposit tracking will retry."
      : /deposit not matched|address_issued/i.test(message)
      ? "Waiting for BTC deposit at this address."
      : `Deposit tracking paused: ${message}`;
    setMessage(friendly, "warn", 1, 15000);
    setTimeout(() => {
      state.depositAutoConfirming = false;
      const progress = confirmationProgressFromUi();
      if (state.depositOwner && progress.current < progress.required) {
        autoConfirmDeposit();
      }
    }, 4000);
    return;
  }
  state.depositAutoConfirming = false;
  const progress = confirmationProgressFromUi();
  if (progress.current < progress.required) {
    setTimeout(() => autoConfirmDeposit(), 4000);
  }
}

async function shieldDeposit(targetBatchKey = state.activeBatchKey || "") {
  state.shieldPending = true;
  const selected = state.batches.find((batch) => batchKey(batch) === String(targetBatchKey || "")) || activeBatch();
  if (selected) {
    activateDepositBatch(selected);
  }
  const batch = selected || activeBatch();
  const depositIndex = Number(batch?.depositIndex || 0);
  updateDashboard();
  setMessage("Shielding deposit into notes...", "");
  try {
    const shieldRef = batch?.depositId || batch?.inboundTxId || "";
    const amountSats = Number(batch?.amountSats || 0) || getAmountSats();
    if (!shieldRef) {
      throw new Error("deposit id is not known yet");
    }
    if (!amountSats) {
      throw new Error("deposit amount is not known yet");
    }
    const seedHex = await walletRootSeedHex();
    $("walletRootStatus").hidden = true;
    $("walletRootStatus").textContent = `Root fingerprint ${await rootFingerprint(seedHex)}. Notes are indexed on your secret phrase.`;
    const receipt = await deriveShieldReceipt(shieldRef, amountSats, seedHex, depositIndex);
    const noteCommitments = commitments(receipt);
    const commitmentAmountSats = noteCommitments.reduce((sum, note) => sum + Number(note.denomination_sats || 0), 0);
    const authorization = await shieldAuthorization(seedHex, shieldRef, commitmentAmountSats, noteCommitments, depositIndex);
    const payload = await api("/thornado/shield", {
      method: "POST",
      body: {
        commitments: noteCommitments,
        deposit_pubkey: authorization.deposit_pubkey,
        signature: authorization.signature,
        deposit_id: shieldRef
      }
    });
    payload.tx_response = await waitForCommittedTx(payload.txhash);
    invalidateShielderSyncCache();
    upsertBatch({
      depositIndex,
      batchId: batchKey(batch),
      owner: batch?.owner || state.depositOwner,
      pubkey: batch?.pubkey || "",
      depositId: shieldRef,
      depositAddress: batch?.depositAddress || $("depositAddress").textContent.trim(),
      amountSats,
      status: "committed",
      receipt,
      shieldedAt: Date.now(),
      maturesAt: Date.now() + DEMO_MATURITY_MS
    });
    state.selectedNote = 0;
    state.waitStartedAt = Date.now();
    state.waitMaturesAt = state.waitStartedAt + DEMO_MATURITY_MS;
    await refreshReceiptPoolPositions();
    renderNotes();
    renderDepositHistory();
    log("shield", { ...payload, receipt });
    const remainderSats = Number(receipt.remainder_sats || 0);
    const remainderMessage = remainderSats > 0
      ? ` ${btcAmount(remainderSats)} dust remainder moved to the fee bucket.`
      : "";
    setMessage(`Minted ${receipt.notes.length} notes.${remainderMessage}`, "ok");
  } finally {
    state.shieldPending = false;
    updateDashboard();
  }
}

async function withdrawNote(noteIndex = state.selectedNote || 0) {
  state.selectedNote = noteIndex;
  const note = selectedReceiptNote();
  const key = noteKey(note);
  state.withdrawingNote = key;
  renderNotes();
  try {
    setMessage("Generating withdrawal proof in this browser...", "", 8, 120000);
    const { proof, public: publicInputs } = await validateGeneratedProof(noteIndex);
    const payload = await api("/thornado/withdraw", {
      method: "POST",
      body: { proof, public: publicInputs }
    });
    payload.tx_response = await waitForCommittedTx(payload.txhash, 45000);
    invalidateShielderSyncCache();
    setMessage("Withdrawal accepted. Waiting for BTC payout...", "ok", 9, 600000);
    updateDashboard();
    const withdrawalID = payload.withdrawal_id || publicInputs.nullifier_hash;
    const redeem = payload.withdrawal_id
      ? await api(`/thornado/shielder/redeem/${payload.withdrawal_id}`).catch(() => null)
      : null;
    const inHash = redeem?.in_hash || withdrawalID;
    state.withdrawnNotes[key] = { txhash: payload.txhash, withdrawalID, inHash, status: "pending" };
    renderNotes();
    const requestedHeight = Number(redeem?.requested_height || 0);
    const netSats = Math.max(0, Number(note.denomination_sats || 0) - Number($("feeSats").value || 0));
    const outHash = await waitForOutboundHash(inHash, $("recipient").value.trim(), netSats, requestedHeight);
    state.withdrawnNotes[key] = { txhash: payload.txhash, withdrawalID, inHash, outHash };
    log("withdraw", { ...payload, out_hash: outHash });
    setMessage(outHash ? "Withdrawal complete. BTC payout found." : "Withdrawal accepted. BTC payout is still pending.", outHash ? "ok" : "warn", 10, 600000);
  } catch (error) {
    if (isSpentNullifierError(error) && note) {
      note.spent = true;
      state.withdrawnNotes[key] = {
        status: "spent",
        nullifierHash: state.lastWithdrawalPublic?.nullifier_hash || note.nullifier_hash || ""
      };
      renderNotes();
    }
    setMessage(`Withdraw failed: ${errorText(error)}`, "error", 10, 60000);
    throw error;
  } finally {
    state.withdrawingNote = null;
    renderNotes();
    updateDashboard();
  }
}

function openWithdrawAddressModal(noteIndex) {
  state.pendingWithdrawNote = noteIndex;
  const input = $("withdrawRecipientInput");
  input.value = "";
  updateWithdrawAddressModal();
  $("withdrawAddressModal").hidden = false;
  setTimeout(() => input.focus(), 0);
}

function closeWithdrawAddressModal() {
  $("withdrawAddressModal").hidden = true;
  state.pendingWithdrawNote = null;
  $("withdrawRecipientInput").value = "";
  updateWithdrawAddressModal();
}

function updateWithdrawAddressModal() {
  const button = $("confirmWithdrawAddress");
  if (!button) {
    return;
  }
  button.disabled = !$("withdrawRecipientInput").value.trim();
}

async function confirmWithdrawAddress() {
  const address = $("withdrawRecipientInput").value.trim();
  if (!address) {
    throw new Error("receive address is required");
  }
  const noteIndex = Number(state.pendingWithdrawNote || 0);
  $("recipient").value = address;
  closeWithdrawAddressModal();
  await withdrawNote(noteIndex);
}

window.thornadoE2E = {
  snapshot() {
    return {
      batches: state.batches.map((batch) => ({
        depositIndex: batch.depositIndex,
        status: batch.status,
        amountSats: batch.amountSats,
        notes: batch.receipt?.notes?.length || 0,
        mature: batchMature(batch)
      })),
      selectedNote: state.selectedNote,
      message: $("message").textContent
    };
  },
  async deriveForSelectedDeposit() {
    const batch = selectedFlowBatch();
    if (!batch) throw new Error("no selected deposit");
    const seedHex = await walletRootSeedHex();
    const receipt = await deriveShieldReceipt(
      batch.depositId || batch.inboundTxId || "",
      Number(batch.amountSats || 0),
      seedHex,
      Number(batch.depositIndex || 0)
    );
    return {
      deposit: depositLabel(batch),
      amount: btcAmount(batch.amountSats || 0),
      notes: receipt.notes.map((note) => btcAmount(note.denomination_sats)),
      remainder: btcAmount(receipt.remainder_sats || 0),
      fees: receipt.notes.map((note) => btcAmount(withdrawalFeeForDenomination(note.denomination_sats)))
    };
  },
  async withdrawFirstMature(recipient) {
    const batch = state.batches.find((item) => item.receipt?.notes?.length && batchMature(item));
    if (!batch) throw new Error("no mature batch");
    state.receipt = batch.receipt;
    state.activeBatchIndex = Number(batch.depositIndex || 0);
    state.selectedNote = Math.max(0, state.receipt.notes.findIndex((note) => !note.spent && !state.withdrawnNotes[noteKey(note)]));
    $("recipient").value = recipient;
    await withdrawNote(state.selectedNote);
    return this.snapshot();
  }
};

async function browserTx(path, body, label, writer = writeNodeStatus) {
  const signer = await nodeSignerAddress();
  const payload = await api(path, {
    method: "POST",
    body: { ...body, signer }
  });
  writer(label, payload);
  return payload;
}

async function refreshNodeTools() {
  const [metrics, auctions, block] = await Promise.all([
    api("/thornado/node/metrics").catch(() => null),
    api("/thornado/node/auctions").catch(() => ({ auctions: [] })),
    api("/thornado/block").catch(() => null)
  ]);
  state.nodeSales = Array.isArray(auctions?.auctions) ? auctions.auctions : [];
  const height = Number(block?.id?.height || block?.header?.height || block?.block?.header?.height || 0);
  if (metrics) {
    $("nodeNextSlot").textContent = String(metrics.next_slot ?? "unknown");
    $("nodeRequiredBond").textContent = metrics.next_slot_bond_required_sats
      ? btcAmount(Number(metrics.next_slot_bond_required_sats))
      : "unknown";
    if ($("nodeContextNextSlot")) $("nodeContextNextSlot").textContent = $("nodeNextSlot").textContent;
    if ($("nodeContextRequiredBond")) $("nodeContextRequiredBond").textContent = $("nodeRequiredBond").textContent;
  }
  updateNodeSecretStatus();
  renderNodeSalesList();
	      log("node/tools/refresh", { metrics, auctions, height });
	    }

function showNodeWorkflow(workflow) {
  state.nodeWorkflow = workflow || "new";
  document.querySelectorAll("[data-node-section]").forEach((section) => {
    section.hidden = section.dataset.nodeSection !== state.nodeWorkflow;
  });
  document.querySelectorAll("[data-node-subtab]").forEach((button) => {
    button.classList.toggle("active", button.dataset.nodeSubtab === state.nodeWorkflow);
  });
  renderEmbeddedNodeFlows();
}

function activeNodeBatch() {
  return state.batches.find((batch) => normalizeDepositType(batch.depositType) === "node" && batchKey(batch) === String(state.activeBatchKey || "")) ||
    state.batches.find((batch) => normalizeDepositType(batch.depositType) === "node" && Number(batch.depositIndex || 0) === Number(state.activeBatchIndex || 0)) ||
    null;
}

function renderEmbeddedFlow(targetId, title) {
  const el = $(targetId);
  if (!el) return;
  const batch = activeNodeBatch();
  const progress = batchConfirmationProgress(batch);
  const address = batch?.depositAddress || $("depositAddress").textContent.trim() || "none";
  const status = batch?.status || "not started";
  const amount = Number(batch?.amountSats || 0);
  el.innerHTML = `
    <div class="row"><span>${escapeHtml(title)}</span><strong>${escapeHtml(status)}</strong></div>
    <div class="row"><span>Address</span><strong>${escapeHtml(short(address, 18, 10))}</strong></div>
    <div class="row"><span>Confirmations</span><strong>${confirmationProgressLabel(progress)}</strong></div>
    <div class="row"><span>Amount</span><strong>${btcAmount(amount)}</strong></div>
  `;
}

function renderEmbeddedNodeFlows() {
  renderEmbeddedFlow("nodeBondFlow", "Bond deposit");
  renderEmbeddedFlow("nodeSalesBidFlow", "Bid deposit");
}

function auctionID(auction) {
  return String(auction?.auction_id || auction?.auctionId || auction?.id || auction?.node_pub_key || auction?.seller_node_pub_key || "");
}

function renderNodeSalesList() {
  const el = $("nodeSalesList");
  if (!el) return;
  el.textContent = "";
  const auctions = state.nodeSales || [];
  if (!auctions.length) {
    const row = document.createElement("div");
    row.className = "pane-empty";
    row.textContent = "No auctions";
    el.append(row);
    return;
  }
  for (const auction of auctions) {
    const id = auctionID(auction);
    const reserve = Number(auction.reserve_sats || auction.reserveSats || 0);
    const expiry = auction.expiry_height || auction.expiryHeight || "unknown";
    const seller = auction.seller_node_pub_key || auction.sellerNodePubKey || auction.node_pub_key || "";
    const card = document.createElement("div");
    card.className = "batch-card";
    card.innerHTML = `
      <div class="batch-meta">
        <div class="row"><span>Auction</span><strong>${escapeHtml(id || "unknown")}</strong></div>
        <div class="row"><span>Seller node</span><strong>${escapeHtml(short(seller || "unknown", 16, 10))}</strong></div>
        <div class="row"><span>Reserve</span><strong>${reserve ? btcAmount(reserve) : "unknown"}</strong></div>
        <div class="row"><span>Expiry</span><strong>${escapeHtml(String(expiry))}</strong></div>
      </div>
    `;
    const action = document.createElement("div");
    action.className = "actions";
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = "Bid";
    button.addEventListener("click", () => {
      state.selectedSaleAuctionId = id;
      $("salesSelectedAuction").textContent = id || "unknown";
      prepareNodeSalesBidDeposit().catch((error) => setMessage(errorText(error), "error"));
    });
    action.append(button);
    card.append(action);
    el.append(card);
  }
}

async function buildBondFromNotesCommand() {
  const operatorPubKey = (await deriveNodeIdentity()).authPubkey;
  const nodePubKey = $("bondNodePubkey").value.trim();
  if (!operatorPubKey || !nodePubKey) {
    throw new Error("node address and operator pubkey are required");
  }
  const candidate = firstProtocolRedeemableNodeNote();
  if (!candidate) {
    throw new Error("shield a node-purpose deposit and wait until its notes are mature before bonding");
  }
  setMessage("Generating bond proof from node-purpose note...", "", 8, 120000);
  const { proof, public: publicInputs } = await validateGeneratedProof(
    candidate.index,
    protocolRedeemOptions("bond_escrow", { node_pub_key: nodePubKey })
  );
  const payload = await browserTx("/thornado/browser/node/bond-from-notes", {
    node_pubkey: nodePubKey,
    operator_pubkey: operatorPubKey,
    proof,
    public: publicInputs
  }, "Bond from shielded notes");
  state.withdrawnNotes[noteKey(candidate.note)] = {
    txhash: payload.txhash,
    withdrawalID: publicInputs.nullifier_hash,
    status: "bond"
  };
  renderNotes();
  updateDashboard();
}

function updateNodeSecretStatus() {
  const connected = Boolean($("walletRoot")?.value.trim());
  const text = connected ? "Secret connected" : "Set secret in User";
  if ($("nodeSecretStatus")) $("nodeSecretStatus").textContent = text;
  if ($("nodeContextSecretStatus")) $("nodeContextSecretStatus").textContent = text;
  return connected;
}

async function deriveNodeIdentity() {
  if (!updateNodeSecretStatus()) {
    throw new Error("set or generate your secret in User first");
  }
  const seedHex = await walletRootSeedHex();
  const authPubkey = await clientPubkeyFromSeed(seedHex, 0, "node");
  const authAddress = await ownerAddressFromCompressedPubkey(authPubkey);
  const nextIndex = Number(state.nextDepositIndexByType.node || 0);
  return { authPubkey, authAddress, nextIndex };
}

async function nodeSignerAddress() {
  return (await deriveNodeIdentity()).authAddress;
}

async function prepareNodePurposeDeposit(label, options = {}) {
  state.depositPurpose = "node";
  state.activeBatchKey = "";
  state.activeBatchIndex = Number(state.nextDepositIndexByType.node || 0);
  syncNodePurposeAmount();
  if (options.showUserTab) {
    showTab("user");
  }
  $("stageDeposit").dataset.expanded = "1";
  $("stageDepositTrack").dataset.expanded = "1";
  await refreshClientPubkey(state.activeBatchIndex).catch(() => null);
  await deriveNodeIdentity();
  setMessage(`${label}: get an address, shield, then unshield to protocol escrow.`, "ok", 1, 30000);
  renderEmbeddedNodeFlows();
  updateDashboard();
}

async function prepareNodeBondDeposit() {
  showNodeWorkflow("bond");
  await prepareNodePurposeDeposit("Bond deposit");
}

async function prepareNodeSalesBidDeposit() {
  showTab("sales");
  await prepareNodePurposeDeposit("Bid deposit");
}

async function requestNodePurposeDeposit() {
  state.depositPurpose = "node";
  syncNodePurposeAmount();
  await requestDeposit();
  renderEmbeddedNodeFlows();
}

async function shieldNodePurposeDeposit() {
  state.depositPurpose = "node";
  await shieldDeposit();
  renderEmbeddedNodeFlows();
}

async function prepareUserPurposeDeposit() {
  state.depositPurpose = "user";
  state.activeBatchKey = "";
  state.activeBatchIndex = Number(state.nextDepositIndexByType.user || 0);
  showTab("user");
  await refreshClientPubkey(state.activeBatchIndex).catch(() => null);
  updateDashboard();
}

function activeNodePurposeAmountInput() {
  if (state.currentTab === "sales") {
    return $("salesBidAmountSats");
  }
  if (state.nodeWorkflow === "bond") {
    return $("nodeBondAmountSats");
  }
  return null;
}

function syncNodePurposeAmount() {
  const input = activeNodePurposeAmountInput();
  const amount = input?.value.trim();
  if (amount) {
    $("amountSats").value = amount;
  }
}

async function prepareNodeEntitlementBucket(label) {
  state.depositPurpose = "node";
  await deriveNodeIdentity();
  writeNodeStatus(`${label} bucket`, {
    status: "ready",
    details: [
    `${label} uses the secret set in User.`,
    `Shield resulting notes into the next node-purpose bucket.`
    ]
  });
}

async function prepareNodeFeeShield() {
  const nodePubKey = $("incomeNodePubkey").value.trim() || $("bondNodePubkey").value.trim() || $("nodeConsPubkey").value.trim();
  await prepareNodeEntitlementBucket("Fee split");
  let entitlement = null;
  if (nodePubKey) {
    entitlement = await api(`/thornado/fee/entitlement/${encodeURIComponent(nodePubKey)}`).catch(() => null);
  }
  const claimable = Number(entitlement?.claimable_sats || 0);
  if ($("nodeIncomeClaimable")) $("nodeIncomeClaimable").textContent = claimable ? btcAmount(claimable) : "query unavailable or zero";
  writeNodeStatus("Fee split", {
    claimable: claimable ? btcAmount(claimable) : "query unavailable or zero"
  });
}

async function prepareNodeSaleShield() {
  await prepareNodeEntitlementBucket("Sale payout");
  const auctionID = $("sellAuctionId").value.trim() || state.selectedSaleAuctionId || "<auction-id>";
  const bidID = $("sellBidId").value.trim() || "<bid-id>";
  writeNodeStatus("Sale payout", {
    auction_id: auctionID,
    bid_id: bidID
  });
}

async function submitNodeSaleShield() {
  const lane = await deriveNodeIdentity();
  const auctionID = $("sellAuctionId").value.trim() || state.selectedSaleAuctionId || "";
  const bidID = $("sellBidId").value.trim();
  const commitments = $("saleSellerCommitments").value.trim();
  const signature = $("saleSellerSignature").value.trim();
  if (!auctionID || !bidID || !commitments || !signature) {
    throw new Error("auction id, bid id, seller commitments, and signature are required");
  }
  await browserTx("/thornado/browser/node/sale-shield", {
    auction_id: auctionID,
    bid_id: bidID,
    commitments: parseListInput(commitments),
    deposit_pubkey: lane.authPubkey,
    signature
  }, "Shield sale payout");
}

async function buildAuctionCreateCommand() {
  const seller = $("saleSellerNodePubkey").value.trim() || "<seller-node-pubkey>";
  const reserve = $("saleReserveSats").value.trim() || "<reserve-sats>";
  const expiry = $("saleExpiryHeight").value.trim() || "<expiry-height>";
  await browserTx("/thornado/browser/node/auction-create", {
    node_pubkey: seller,
    reserve_sats: reserve,
    expiry_height: expiry
  }, "List node slot for sale");
}

async function buildAuctionSelectCommand() {
  const auctionID = $("sellAuctionId").value.trim() || state.selectedSaleAuctionId || "<auction-id>";
  const bidID = $("sellBidId").value.trim() || "<bid-id>";
  await browserTx("/thornado/browser/node/auction-select-bid", {
    auction_id: auctionID,
    bid_id: bidID
  }, "Select node sale bid");
}

async function buildAuctionBidCommand() {
  const selectedText = $("salesSelectedAuction").textContent.trim();
  const auctionID = state.selectedSaleAuctionId || (selectedText && selectedText !== "none" ? selectedText : "") || "<auction-id>";
  const operatorPubKey = $("salesBidOperatorPubkey").value.trim() || "<operator-pubkey>";
  const nodePubKey = $("salesBidNodePubkey").value.trim() || "<node-pubkey>";
  await browserTx("/thornado/browser/node/auction-bid-create", {
    auction_id: auctionID,
    operator_pubkey: operatorPubKey,
    node_pubkey: nodePubKey
  }, "Create node sale bid", writeSalesStatus);
}

async function fundAuctionBidFromNotes() {
  const bidID = $("salesBidId").value.trim();
  if (!bidID) {
    throw new Error("bid id is required");
  }
  const candidate = firstProtocolRedeemableNodeNote();
  if (!candidate) {
    throw new Error("shield a node-purpose deposit and wait until its notes are mature before funding the bid");
  }
  setMessage("Generating bid funding proof from node-purpose note...", "", 8, 120000);
  const { proof, public: publicInputs } = await validateGeneratedProof(
    candidate.index,
    protocolRedeemOptions("bid_deposit", { bid_id: bidID })
  );
  const payload = await api("/thornado/withdraw", {
    method: "POST",
    body: { proof, public: publicInputs }
  });
  payload.tx_response = await waitForCommittedTx(payload.txhash, 45000);
  invalidateShielderSyncCache();
  writeSalesStatus("Fund node sale bid", payload);
  state.withdrawnNotes[noteKey(candidate.note)] = {
    txhash: payload.txhash,
    withdrawalID: payload.withdrawal_id || publicInputs.nullifier_hash,
    status: "bid"
  };
  renderNotes();
  updateDashboard();
}

function writeNodeStatus(label, payload) {
  $("nodeCommandOutput").hidden = false;
  $("nodeCommandOutput").textContent = `${label}\n\n${JSON.stringify(payload, null, 2)}`;
  if (state.nodeWorkflow === "income" && $("nodeIncomeOutput")) {
    $("nodeIncomeOutput").textContent = `${label}\n\n${JSON.stringify(payload, null, 2)}`;
  }
  if (state.nodeWorkflow === "sell" && $("nodeSellOutput")) {
    $("nodeSellOutput").textContent = `${label}\n\n${JSON.stringify(payload, null, 2)}`;
  }
  log("node/action", { label, payload });
}

function writeSalesStatus(label, payload) {
  $("nodeSalesCommandOutput").hidden = false;
  $("nodeSalesCommandOutput").textContent = `${label}\n\n${JSON.stringify(payload, null, 2)}`;
  log("node/sales/action", { label, payload });
}

	    function nodeKeyName() {
	      return $("nodeKeyName").value.trim() || "node";
	    }

	    async function buildSetIpCommand() {
	      const ip = $("nodeIpAddress").value.trim();
	      if (!ip) {
	        throw new Error("ip address is required");
	      }
	      await browserTx("/thornado/browser/node/set-ip", { ip }, "Set IP");
	    }

	    async function buildSetVersionCommand() {
	      await browserTx("/thornado/browser/node/set-version", {}, "Set version");
	    }

	    async function buildSetKeysCommand() {
	      const secp = $("nodeSecpPubkey").value.trim();
	      const ed = $("nodeEdPubkey").value.trim();
	      const cons = $("nodeConsPubkey").value.trim();
	      if (!secp || !ed || !cons) {
	        throw new Error("secp256k1, ed25519, and consensus pubkeys are required");
	      }
	      await browserTx("/thornado/browser/node/set-keys", {
	        secp_pubkey: secp,
	        ed_pubkey: ed,
	        consensus_pubkey: cons
	      }, "Set node keys");
	    }

	    async function buildProposeUpgradeCommand() {
	      const name = $("upgradeName").value.trim();
	      const height = $("upgradeHeight").value.trim();
	      const info = $("upgradeInfo").value.trim();
	      if (!name || !height) {
	        throw new Error("upgrade name and height are required");
	      }
	      await browserTx("/thornado/browser/upgrade/propose", { name, height, info }, "Propose upgrade");
	    }

	    async function buildApproveUpgradeCommand() {
	      const name = $("upgradeName").value.trim();
	      if (!name) {
	        throw new Error("upgrade name is required");
	      }
	      await browserTx("/thornado/browser/upgrade/approve", { name }, "Approve upgrade");
	    }

	    async function buildRejectUpgradeCommand() {
	      const name = $("upgradeName").value.trim();
	      if (!name) {
	        throw new Error("upgrade name is required");
	      }
	      await browserTx("/thornado/browser/upgrade/reject", { name }, "Reject upgrade");
	    }

	    async function buildShieldFeesCommand() {
	      const nodePubKey = $("incomeNodePubkey").value.trim() || $("bondNodePubkey").value.trim() || $("nodeConsPubkey").value.trim();
	      const sig = $("incomeFeeOperatorSignature").value.trim();
	      const commitments = $("incomeFeeCommitments").value.trim();
	      const pubkeys = $("incomeFeeNotePubkeys").value.trim();
	      if (!nodePubKey || !sig || !commitments || !pubkeys) {
	        throw new Error("node pubkey, operator signature, commitments, and fee note pubkeys are required");
	      }
	      await browserTx("/thornado/browser/node/shield-fees", {
	        node_pubkey: nodePubKey,
	        operator_signature: sig,
	        commitments: parseListInput(commitments),
	        fee_note_pubkeys: parseListInput(pubkeys)
	      }, "Shield fees");
	    }

	    async function buildPauseCommand() {
	      await browserTx("/thornado/browser/node/pause-chain", {}, "Pause chain");
	    }

	    async function buildResumeCommand() {
	      await browserTx("/thornado/browser/node/resume-chain", {}, "Resume chain");
	    }

async function run(action) {
  setMessage("");
  try {
    await action();
  } catch (error) {
    const message = error?.message || String(error);
    $("requestDeposit").disabled = false;
    $("powStatus").hidden = true;
    setMessage(message, "error");
    log("error", { error: message });
  }
}

function revealSecret() {
  state.secretMode = "revealed";
  renderSecret();
}

function hideSecret() {
  state.secretMode = "hidden";
  $("customSecretError").hidden = true;
  $("customSecretError").textContent = "";
  setMessage("");
  renderSecret();
}

const flowHelp = {
  deposit: {
    title: "Get Address",
    src: "/thornado/ui/how-it-works/get-address.png?v=shield-minimal-v1",
    alt: "Get Address data flow between user, browser, and protocol"
  },
  wait: {
    title: "Deposit",
    src: "/thornado/ui/how-it-works/deposit.png?v=shield-minimal-v1",
    alt: "Deposit data flow between user, Bitcoin, and protocol"
  },
  shield: {
    title: "Shield",
    src: "/thornado/ui/how-it-works/shield.png?v=shield-minimal-v1",
    alt: "Shield data flow showing where the privacy link breaks"
  },
  withdraw: {
    title: "Withdraw",
    src: "/thornado/ui/how-it-works/withdraw.png?v=shield-minimal-v1",
    alt: "Withdraw data flow showing local note scan, ZK proof, and payout"
  }
};

function openFlowHelp(key) {
  const help = flowHelp[key];
  if (!help) return;
  $("flowModalTitle").textContent = help.title;
  $("flowModalImage").src = help.src;
  $("flowModalImage").alt = help.alt;
  $("flowModal").hidden = false;
}

function closeFlowHelp() {
  $("flowModal").hidden = true;
  $("flowModalImage").src = "";
}

function startCustomSecret() {
  state.secretMode = "custom";
  $("customWalletRoot").value = "";
  $("customSecretError").hidden = true;
  $("customSecretError").textContent = "";
  renderSecret();
  $("customWalletRoot").focus();
}

function cancelCustomSecret() {
  $("customWalletRoot").value = "";
  $("customSecretError").hidden = true;
  $("customSecretError").textContent = "";
  state.secretMode = "revealed";
  setMessage("");
  renderSecret();
}

async function saveCustomSecret() {
  $("customSecretError").hidden = true;
  $("customSecretError").textContent = "";
  let words;
  try {
    words = await validateMnemonic($("customWalletRoot").value, [12, 24]);
  } catch (error) {
    $("customSecretError").textContent = error.message;
    $("customSecretError").hidden = false;
    throw error;
  }
  $("walletRoot").value = words.join(" ");
  $("customWalletRoot").value = "";
  state.secretMode = "hidden";
  await persistWalletSecret();
  resetDepositState();
  await refreshChurnWindow().catch((error) => log("churn/window", { error: error.message }));
  updateNodeSecretStatus();
  updateDashboard();
  setMessage("Syncing deposits from this secret...", "", 1, 10000);
  await discoverDepositBatches({ recoverNotes: false });
  if (state.batches.length) {
    setMessage("Deposits synced.", "ok", 1, 10000);
  } else {
    setMessage("No previous deposits found. Get Address will request the next address.", "", 1, 10000);
  }
  updateDashboard();
}

async function copyText(text) {
  let copied = false;
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      copied = true;
    }
  } catch {
    copied = false;
  }
  if (!copied) {
    const scratch = document.createElement("textarea");
    scratch.value = text;
    scratch.setAttribute("readonly", "");
    scratch.style.position = "fixed";
    scratch.style.top = "-1000px";
    document.body.append(scratch);
    scratch.select();
    try {
      copied = document.execCommand("copy");
    } catch {
      copied = false;
    }
    scratch.remove();
  }
  return copied;
}

async function copyDepositAddress() {
  if (depositExpired()) {
    throw new Error("Deposit address expired. Request a new address for the current churn cycle.");
  }
  const address = $("depositAddress").textContent.trim();
  if (!address) {
    return;
  }
  const copied = await copyText(address);
  $("copyDepositAddress").textContent = copied ? "Copied" : "Copy blocked";
  setMessage(copied ? "Deposit address copied." : "Clipboard blocked by the browser. Select the address manually.", copied ? "ok" : "warn");
  setTimeout(() => {
    $("copyDepositAddress").textContent = "Copy";
  }, 1200);
}

async function copySecret() {
  const secret = $("walletRoot").value.trim();
  if (!secret) {
    return;
  }
  const copied = await copyText(secret);
  $("copySecret").textContent = copied ? "Copied" : "Copy blocked";
  setMessage(copied ? "Secret copied." : "Clipboard blocked by the browser. Select the secret manually.", copied ? "ok" : "warn");
  setTimeout(() => {
    $("copySecret").textContent = "Copy";
  }, 1200);
}

function closeStatusMenus() {
  ["routeStatus", "nodeStatus"].forEach((id) => {
    const menu = $(`${id}Menu`);
    const button = $(`${id}Button`);
    if (menu) {
      menu.hidden = true;
    }
    if (button) {
      button.setAttribute("aria-expanded", "false");
    }
  });
}

function toggleStatusMenu(id) {
  const menu = $(`${id}Menu`);
  const button = $(`${id}Button`);
  if (!menu || !button) {
    return;
  }
  const willOpen = menu.hidden;
  closeStatusMenus();
  menu.hidden = !willOpen;
  button.setAttribute("aria-expanded", willOpen ? "true" : "false");
}

function switchToTor() {
  if (!state.onionAddress) {
    return;
  }
  const raw = state.onionAddress.match(/^https?:\/\//)
    ? state.onionAddress
    : `http://${state.onionAddress}`;
  const target = new URL(raw);
  if (!target.pathname || target.pathname === "/") {
    target.pathname = location.pathname;
  }
  if (!target.search) {
    target.search = location.search;
  }
  window.open(target.href, "_blank", "noopener");
}

	    function showTab(tabName) {
	      const isNetwork = tabName === "network";
	      const isNodes = tabName === "nodes";
	      const isSales = tabName === "sales";
	      const isHow = tabName === "how";
    state.currentTab = isNetwork ? "network" : isNodes ? "nodes" : isSales ? "sales" : isHow ? "how" : "user";
    if (state.currentTab !== "user" && state.secretMode !== "hidden") {
      state.secretMode = "hidden";
      $("customSecretError").hidden = true;
      $("customSecretError").textContent = "";
      renderSecret();
    }
	      $("userPanel").hidden = isNetwork || isNodes || isSales || isHow;
	      $("networkPanel").hidden = !isNetwork;
	      $("nodesPanel").hidden = !isNodes;
	      $("nodeSalesPanel").hidden = !isSales;
	      $("howPanel").hidden = !isHow;
	      $("tabUser").classList.toggle("active", !isNetwork && !isNodes && !isSales && !isHow);
	      $("tabNodes").classList.toggle("active", isNodes);
	      $("tabNodeSales").classList.toggle("active", isSales);
	      $("tabNetwork").classList.toggle("active", isNetwork);
	      $("tabHow").classList.toggle("active", isHow);
	      if (isNetwork || isNodes || isSales) {
	        refreshNodeTools().catch((error) => log("node/tools/refresh", { error: error.message }));
	      }
    if (isNodes) {
      showNodeWorkflow(state.nodeWorkflow || "new");
    }
    if (quoteWriteTimer) {
      clearTimeout(quoteWriteTimer);
      quoteWriteTimer = null;
    }
    if (moreRevealTimer) {
      clearTimeout(moreRevealTimer);
      moreRevealTimer = null;
    }
    state.moreOpen = false;
    state.moreSettled = false;
    state.quoteWriting = false;
    closeStatusMenus();
    renderMoreNav();
    renderIntroGate();
	    }

	    $("tabUser").addEventListener("click", () => run(prepareUserPurposeDeposit));
	    $("tabNodes").addEventListener("click", () => showTab("nodes"));
	    $("tabNodeSales").addEventListener("click", () => showTab("sales"));
	    $("tabNetwork").addEventListener("click", () => showTab("network"));
	    $("tabHow").addEventListener("click", () => showTab("how"));
$("moreToggle").addEventListener("click", toggleMoreNav);
$("routeStatusButton").addEventListener("click", (event) => {
  event.stopPropagation();
  toggleStatusMenu("routeStatus");
});
$("nodeStatusButton").addEventListener("click", (event) => {
  event.stopPropagation();
  toggleStatusMenu("nodeStatus");
});
$("switchTor").addEventListener("click", (event) => {
  event.stopPropagation();
  switchToTor();
  closeStatusMenus();
});
$("startFlow").addEventListener("click", () => markAppStarted(true));
$("requestDeposit").addEventListener("click", () => run(requestDeposit));
$("depositInfoButton").addEventListener("click", () => openFlowHelp("deposit"));
$("depositTrackInfoButton").addEventListener("click", () => openFlowHelp("wait"));
$("shieldInfoButton").addEventListener("click", () => openFlowHelp("shield"));
$("waitInfoButton").addEventListener("click", () => openFlowHelp("wait"));
$("withdrawInfoButton").addEventListener("click", () => openFlowHelp("withdraw"));
$("flowModalClose").addEventListener("click", closeFlowHelp);
$("flowModal").addEventListener("click", (event) => {
  if (event.target === $("flowModal")) closeFlowHelp();
});
window.addEventListener("keydown", (event) => {
  if (event.key === "Escape" && !$("flowModal").hidden) closeFlowHelp();
  if (event.key === "Escape" && !$("withdrawAddressModal").hidden) closeWithdrawAddressModal();
});
document.querySelectorAll("[data-stage-toggle]").forEach((button) => {
  button.addEventListener("click", () => toggleStage(button.dataset.stageToggle));
});
document.addEventListener("click", (event) => {
  const tabButton = event.target.closest?.("#tabUser, #tabNodes, #tabNodeSales, #tabNetwork, #tabHow");
  if (tabButton) {
    event.preventDefault();
    event.stopImmediatePropagation();
    if (tabButton.id === "tabUser") {
      run(prepareUserPurposeDeposit);
    } else if (tabButton.id === "tabNodes") {
      showTab("nodes");
    } else if (tabButton.id === "tabNodeSales") {
      showTab("sales");
    } else if (tabButton.id === "tabNetwork") {
      showTab("network");
    } else if (tabButton.id === "tabHow") {
      showTab("how");
    }
    return;
  }
  const option = event.target.closest?.(".deposit-option[data-dropdown-option]");
  if (option) {
    event.preventDefault();
    event.stopImmediatePropagation();
    const paneKey = option.dataset.dropdownOption || "deposit";
    const key = option.dataset.batchKey || "";
    const batch = state.batches.find((item) => batchKey(item) === String(key));
    if (batch) {
      state.paneBatchKeys[paneKey] = batchKey(batch);
      if (paneKey === "deposit") {
        activateDepositBatch(batch);
        state.noteRecoveryStatus = "idle";
        state.noteRecoveryBatchKey = batchKey(batch);
        queueDepositRecovery();
        renderDepositHistory();
      }
    }
    state.openBatchDropdown = "";
    document.querySelectorAll(".deposit-batches-dropdown.open").forEach((item) => {
      item.classList.remove("open");
      item.querySelector(".deposit-selected")?.setAttribute("aria-expanded", "false");
    });
    updateDashboard();
    return;
  }
  const toggle = event.target.closest?.(".deposit-selected[data-dropdown-toggle]");
  if (toggle) {
    event.preventDefault();
    event.stopImmediatePropagation();
    const paneKey = toggle.dataset.dropdownToggle || "deposit";
    const dropdown = toggle.closest(".deposit-batches-dropdown");
    const shouldOpen = state.openBatchDropdown !== paneKey;
    document.querySelectorAll(".deposit-batches-dropdown.open").forEach((item) => {
      item.classList.remove("open");
      item.querySelector(".deposit-selected")?.setAttribute("aria-expanded", "false");
    });
    state.openBatchDropdown = shouldOpen ? paneKey : "";
    if (shouldOpen) {
      dropdown?.classList.add("open");
      toggle.setAttribute("aria-expanded", "true");
    } else {
      toggle.setAttribute("aria-expanded", "false");
    }
  }
}, true);
document.addEventListener("click", () => {
  closeStatusMenus();
  state.openBatchDropdown = "";
  document.querySelectorAll(".deposit-batches-dropdown.open").forEach((item) => {
    item.classList.remove("open");
    item.querySelector(".deposit-selected")?.setAttribute("aria-expanded", "false");
  });
});
$("revealSecret").addEventListener("click", revealSecret);
$("useCustomSecret").addEventListener("click", startCustomSecret);
$("doneSecret").addEventListener("click", hideSecret);
$("cancelCustomSecret").addEventListener("click", cancelCustomSecret);
$("saveCustomSecret").addEventListener("click", () => run(saveCustomSecret));
$("copySecret").addEventListener("click", () => run(copySecret));
$("copyDepositAddress").addEventListener("click", () => run(copyDepositAddress));
$("shieldDeposit").addEventListener("click", () => run(shieldDeposit));
$("validateProof").addEventListener("click", () => run(validateGeneratedProof));
	    $("withdrawNote").addEventListener("click", () => run(withdrawNote));
$("cancelWithdrawAddress").addEventListener("click", closeWithdrawAddressModal);
$("withdrawAddressModal").addEventListener("click", (event) => {
  if (event.target === $("withdrawAddressModal")) closeWithdrawAddressModal();
});
$("confirmWithdrawAddress").addEventListener("click", () => run(confirmWithdrawAddress));
$("withdrawRecipientInput").addEventListener("input", updateWithdrawAddressModal);
$("withdrawRecipientInput").addEventListener("keydown", (event) => {
  if (event.key === "Enter" && !$("confirmWithdrawAddress").disabled) {
    event.preventDefault();
    run(confirmWithdrawAddress);
  }
});
$("refreshNodeTools").addEventListener("click", () => run(refreshNodeTools));
document.querySelectorAll("[data-node-subtab]").forEach((button) => {
  button.addEventListener("click", () => showNodeWorkflow(button.dataset.nodeSubtab));
});
$("prepareNodeBondDeposit").addEventListener("click", () => run(prepareNodeBondDeposit));
$("nodeBondGetAddress").addEventListener("click", () => run(requestNodePurposeDeposit));
$("nodeBondShield").addEventListener("click", () => run(shieldNodePurposeDeposit));
$("buildBondFromNotesCommand").addEventListener("click", () => run(buildBondFromNotesCommand));
	    $("buildSetIpCommand").addEventListener("click", () => run(buildSetIpCommand));
	    $("buildSetVersionCommand").addEventListener("click", () => run(buildSetVersionCommand));
	    $("buildSetKeysCommand").addEventListener("click", () => run(buildSetKeysCommand));
	    $("buildProposeUpgradeCommand").addEventListener("click", () => run(buildProposeUpgradeCommand));
	    $("buildApproveUpgradeCommand").addEventListener("click", () => run(buildApproveUpgradeCommand));
	    $("buildRejectUpgradeCommand").addEventListener("click", () => run(buildRejectUpgradeCommand));
$("nodeIncomePrepareFee").addEventListener("click", () => run(prepareNodeFeeShield));
$("nodeIncomeBuildFee").addEventListener("click", () => run(buildShieldFeesCommand));
$("buildAuctionCreateCommand").addEventListener("click", () => run(buildAuctionCreateCommand));
$("buildAuctionSelectCommand").addEventListener("click", () => run(buildAuctionSelectCommand));
$("nodeSellPreparePayout").addEventListener("click", () => run(prepareNodeSaleShield));
$("nodeSellBuildPayout").addEventListener("click", () => run(submitNodeSaleShield));
$("refreshNodeSales").addEventListener("click", () => run(refreshNodeTools));
$("nodeSalesBidDeposit").addEventListener("click", () => run(prepareNodeSalesBidDeposit));
$("nodeSalesBidGetAddress").addEventListener("click", () => run(requestNodePurposeDeposit));
$("nodeSalesBidShield").addEventListener("click", () => run(shieldNodePurposeDeposit));
$("buildAuctionBidCommand").addEventListener("click", () => run(buildAuctionBidCommand));
$("fundAuctionBidCommand").addEventListener("click", () => run(fundAuctionBidFromNotes));
	    $("buildPauseCommand").addEventListener("click", () => run(buildPauseCommand));
	    $("buildResumeCommand").addEventListener("click", () => run(buildResumeCommand));
	    $("amountSats").addEventListener("input", updateDashboard);
$("feeSats").addEventListener("input", () => {
  state.lastWithdrawalProof = null;
  state.lastWithdrawalPublic = null;
  state.lastWithdrawalFeeSats = null;
  state.lastWithdrawalNoteKey = null;
  state.lastWithdrawalContextKey = "";
  updateDashboard();
});
$("walletRoot").addEventListener("input", () => {
  resetDepositState();
  persistWalletSecret().catch((error) => log("secret/save", { error: error.message }));
  updateNodeSecretStatus();
  updateDashboard();
});
$("walletPassphrase").addEventListener("input", () => {
  resetDepositState();
  persistWalletSecret().catch((error) => log("secret/save", { error: error.message }));
  updateNodeSecretStatus();
  updateDashboard();
});

const urlParams = new URLSearchParams(location.search);
const debugEvents = urlParams.get("debug") === "1";
$("userPanel").classList.toggle("no-debug", !debugEvents);
if (debugEvents) {
  $("debugEventsPanel").hidden = false;
  window.__thornadoTest = {
    proveWithdrawalInWorker
  };
}
if (urlParams.has("recipient")) {
  $("recipient").value = urlParams.get("recipient") || "";
}
state.appStarted = localStorage.getItem(APP_STARTED_KEY) === "1";
renderStatusControls();
renderIntroGate();
renderMoreNav();

(async () => {
  let restoredSecret = false;
  if (urlParams.has("e2eSecret")) {
    const words = await validateMnemonic(urlParams.get("e2eSecret") || "", [12, 24]);
    $("walletRoot").value = words.join(" ");
    $("walletPassphrase").value = "";
    state.secretMode = "hidden";
    await persistWalletSecret();
    restoredSecret = true;
  } else {
    restoredSecret = await restoreWalletSecret();
  }
  if (!restoredSecret) {
    await generateWalletRoot();
  } else {
    markAppStarted(false);
  }
  renderNodeEndpoint();
  updateNodeSecretStatus();
  await refreshChurnWindow().catch((error) => log("churn/window", { error: error.message }));
  renderNotes();
  updateDashboard();
  if (restoredSecret) {
    setMessage("Syncing deposits from this secret...", "", 1, 10000);
    setTimeout(() => {
      discoverDepositBatches({ recoverNotes: false })
        .then(() => {
          updateDashboard();
          if (hasRecoverableDepositBatch()) {
            setMessage("Deposits synced. Searching commitments...", "ok", 1, 10000);
            queueDepositRecovery();
          } else {
            setMessage(state.batches.length ? "Deposits synced." : "No previous deposits found.", state.batches.length ? "ok" : "", 1, 10000);
          }
        })
        .catch((error) => {
          const message = errorText(error);
          log("deposit/startup-scan", { error: message });
          setMessage(`Startup sync skipped: ${message}`, "warn", 1, 15000);
        });
    }, 0);
  } else {
    setMessage("Ready. Click Get Address to request the first address.", "");
  }
})().catch((error) => setMessage(errorText(error), "error"));
setInterval(() => {
  const hasMaturingNotes = state.waitMaturesAt || state.batches.some((batch) => batch.receipt?.notes?.length && batchMaturityMs(batch) > 0);
  if (hasMaturingNotes && $("withdrawAddressModal").hidden) {
    renderNotes();
  }
  if (hasMaturingNotes || state.depositExpiresAt) {
    updateDashboard();
  }
}, 1000);
setInterval(() => {
  const progress = confirmationProgressFromUi();
  const batch = activeBatch();
  const hasTrackableDeposit = Boolean(state.depositOwner && (state.activeIntentId || batch?.pubkey || batch?.depositId));
  if (hasTrackableDeposit && !state.depositAutoConfirming && progress.current < progress.required) {
    autoConfirmDeposit();
  }
}, 5000);
setInterval(() => {
  if (!state.receipt?.notes?.length) {
    return;
  }
  refreshReceiptPoolPositions()
    .then(() => {
      renderNotes();
      updateDashboard();
    })
    .catch((error) => log("notes/pool", { error: error?.message || String(error) }));
}, POOL_REFRESH_MS);
refreshHash().catch((error) => setMessage(error.message, "error"));
refreshNodeCount().catch((error) => log("node/count", { error: error?.message || String(error) }));
setInterval(() => {
  refreshHash().catch((error) => log("block/status", { error: error?.message || String(error) }));
}, 5000);
setInterval(() => {
  refreshNodeCount().catch((error) => log("node/count", { error: error?.message || String(error) }));
}, 15000);
