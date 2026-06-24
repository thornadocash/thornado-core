import qrcode from "./vendor/qrcode.min.js?v=esm";
import { nodeOrigin, requestJson } from "./modules/api.js";
import { parseListInput } from "./modules/forms.js";

let thornadoWasmReady;

async function thornadoWasm() {
  if (!thornadoWasmReady) {
    const wasmVersion = new URLSearchParams(window.location.search).get("v") || "local";
    const wasmUrl = new URL(`./wasm/thornado_web_wasm.js?v=${encodeURIComponent(wasmVersion)}`, import.meta.url).href;
    const wasmBinaryUrl = new URL(`./wasm/thornado_web_wasm_bg.wasm?v=${encodeURIComponent(wasmVersion)}`, import.meta.url).href;
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
    feeClaimAuthorizationForDepositTypeJson: mod.fee_claim_authorization_for_deposit_type_json,
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
const SHIELDER_SYNC_PAGE_LIMIT = 1000;
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
  latestBlockTimeMs: null,
  latestBlockSeenAtMs: null,
  observedBlockMs: null,
  withdrawingNote: null,
  withdrawnNotes: {},
  publicNoteBuckets: {},
  publicNoteBucketsPending: false,
  refundTxByInHash: new Map(),
  refundTxoutPending: false,
  shielderSyncCache: null,
  shielderSyncPending: null,
  shielderSyncPendingFromHeight: 0,
  churnCycleMs: DEFAULT_CHURN_CYCLE_MS,
  churnServerDeltaMs: 0,
  waitStartedAt: null,
  waitMaturesAt: null,
  secretMode: "hidden",
  walletBirthdayMs: 0,
  walletBirthdayHeight: 0,
  lastWithdrawalProof: null,
  lastWithdrawalPublic: null,
  lastWithdrawalFeeSats: null,
  lastWithdrawalNoteKey: null,
  lastWithdrawalContextKey: "",
  withdrawalPayouts: {},
  withdrawAddressMode: "withdraw",
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
  shieldedSummaryOpen: false,
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
  stageUserToggled: {},
  stageSelectionKey: "",
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

function shieldedNoteItems() {
  const items = [];
  const seen = new Set();
  for (const batch of state.batches || []) {
    for (const note of batch?.receipt?.notes || []) {
      const key = noteKey(note);
      if (seen.has(key)) {
        continue;
      }
      seen.add(key);
      const withdrawal = state.withdrawnNotes[key];
      const spent = note.spent || withdrawal?.status === "spent" || Boolean(withdrawal?.outHash);
      items.push({
        batch,
        note,
        key,
        denomination: Number(note.denomination_sats || 0),
        mature: batchMature(batch),
        spent
      });
    }
  }
  return items;
}

function firstTopbarWithdrawCandidate(items = shieldedNoteItems()) {
  return items.find((item) => item.mature && !item.spent) ||
    items.find((item) => !item.spent) ||
    items[0] ||
    null;
}

function renderShieldedSummary() {
  const root = $("shieldedSummary");
  const button = $("shieldedSummaryButton");
  const amount = $("shieldedSummaryAmount");
  const menu = $("shieldedSummaryMenu");
  const list = $("shieldedSummaryList");
  const withdraw = $("shieldedSummaryWithdraw");
  if (!root || !button || !amount || !menu || !list || !withdraw) {
    return;
  }
  const items = shieldedNoteItems();
  const spendable = items.filter((item) => !item.spent);
  const total = spendable.reduce((sum, item) => sum + item.denomination, 0);
  root.hidden = total <= 0;
  if (root.hidden) {
    menu.hidden = true;
    button.setAttribute("aria-expanded", "false");
    return;
  }
  amount.textContent = btcAmount(total);
  menu.hidden = !state.shieldedSummaryOpen;
  button.setAttribute("aria-expanded", state.shieldedSummaryOpen ? "true" : "false");
  list.textContent = "";
  for (const item of spendable) {
    const row = document.createElement("div");
    row.className = "shielded-summary-row";
    row.innerHTML = `<span>${btcAmount(item.denomination)}</span><strong>${item.mature ? "Mature" : "Maturing"}</strong>`;
    list.append(row);
  }
  const candidate = firstTopbarWithdrawCandidate(spendable);
  withdraw.disabled = !candidate;
}

function openTopbarWithdraw() {
  const candidate = firstTopbarWithdrawCandidate();
  if (!candidate?.batch) {
    return;
  }
  markAppStarted(false);
  state.currentTab = "user";
  state.paneBatchKeys.deposit = batchKey(candidate.batch);
  activateDepositBatch(candidate.batch);
  state.receipt = candidate.batch.receipt;
  state.activeBatchKey = batchKey(candidate.batch);
  state.activeBatchIndex = Number(candidate.batch.depositIndex || 0);
  state.selectedNote = Math.max(0, candidate.batch.receipt.notes.findIndex((note) => noteKey(note) === candidate.key));
  state.stageUserToggled.stageWithdraw = true;
  $("stageWithdraw").dataset.expanded = "1";
  state.shieldedSummaryOpen = false;
  showTab("user");
  updateDashboard();
  $("stageWithdraw")?.scrollIntoView({ behavior: "smooth", block: "center" });
}

function openTopbarRevealSecret() {
  markAppStarted(false);
  state.shieldedSummaryOpen = false;
  state.stageUserToggled.stageDeposit = true;
  $("stageDeposit").dataset.expanded = "1";
  showTab("user");
  revealSecret();
  updateDashboard();
  $("secretBox")?.scrollIntoView({ behavior: "smooth", block: "center" });
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

function txOutpointLink(txid, vout, outpoint = "") {
  const value = String(txid || "").trim();
  if (!value) {
    return "none";
  }
  const parsedVout = Number(vout);
  const suffix = Number.isInteger(parsedVout) && parsedVout >= 0
    ? `:${parsedVout}`
    : String(outpoint || "").startsWith(`${value}:`)
      ? `:${String(outpoint).slice(value.length + 1)}`
      : "";
  const safeTitle = escapeHtml(suffix ? `${value}${suffix}` : value);
  const label = `${short(value, 6, 8)}${suffix}`;
  return `<a class="tx-link" href="${btcExplorerUrl(value)}" target="_blank" rel="noopener" title="${safeTitle}">${escapeHtml(label)}</a>`;
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

function autoSetStageExpanded(stageId, expanded) {
  if (state.stageUserToggled?.[stageId]) {
    return;
  }
  $(stageId).dataset.expanded = expanded ? "1" : "0";
}

function toggleStage(stageId) {
  const el = $(stageId);
  const nextExpanded = el.dataset.expanded === "1" ? "0" : "1";
  el.dataset.expanded = nextExpanded;
  state.stageUserToggled[stageId] = true;
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
    return msUntilBlockHeight(expiryHeight);
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
    return msUntilBlockHeight(state.depositExpiresAtHeight);
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

function interpolatedBlockHeight() {
  const height = Number(state.latestBlockHeight);
  const blockMs = Number(state.observedBlockMs || blocksToMs(1));
  const seenAt = Number(state.latestBlockSeenAtMs || 0);
  if (!Number.isFinite(height) || !Number.isFinite(blockMs) || blockMs <= 0 || !seenAt) {
    return height;
  }
  return height + Math.max(0, Date.now() - seenAt) / blockMs;
}

function walletBirthdayWord(ms = state.walletBirthdayMs) {
  const timestamp = Number(ms || 0);
  if (!Number.isFinite(timestamp) || timestamp <= 0) {
    return "000000";
  }
  const date = new Date(timestamp);
  const dd = String(date.getDate()).padStart(2, "0");
  const mm = String(date.getMonth() + 1).padStart(2, "0");
  const yy = String(date.getFullYear() % 100).padStart(2, "0");
  return `${dd}${mm}${yy}`;
}

function setWalletBirthdayNow(force = false) {
  if (force || !state.walletBirthdayMs) {
    state.walletBirthdayMs = Date.now();
  }
  if (force || !state.walletBirthdayHeight) {
    const height = Math.floor(interpolatedBlockHeight());
    state.walletBirthdayHeight = Number.isFinite(height) && height > 0 ? Math.max(1, height - 2) : 0;
  }
}

function walletBirthdayStartHeight() {
  const storedHeight = Number(state.walletBirthdayHeight || 0);
  if (Number.isFinite(storedHeight) && storedHeight > 0) {
    return Math.max(1, Math.floor(storedHeight));
  }
  const birthdayMs = Number(state.walletBirthdayMs || 0);
  const currentHeight = interpolatedBlockHeight();
  const blockMs = Number(state.observedBlockMs || blocksToMs(1));
  if (!Number.isFinite(birthdayMs) || birthdayMs <= 0 || !Number.isFinite(currentHeight) || !Number.isFinite(blockMs) || blockMs <= 0) {
    return 0;
  }
  const blocksSince = Math.max(0, (Date.now() - birthdayMs) / blockMs);
  return Math.max(1, Math.floor(currentHeight - blocksSince) - 2);
}

function msUntilBlockHeight(height) {
  const target = Number(height);
  const current = interpolatedBlockHeight();
  if (!Number.isFinite(target) || !Number.isFinite(current)) {
    return null;
  }
  return blocksToMs(Math.max(0, target - current));
}

function applyObservedBlockMs(blockMs) {
  if (!Number.isFinite(blockMs) || blockMs <= 0) return;
  const clamped = Math.min(10 * 60 * 1000, Math.max(250, blockMs));
  state.observedBlockMs = state.observedBlockMs
    ? (state.observedBlockMs * 0.7) + (clamped * 0.3)
    : clamped;
  state.blocksPerYear = Math.round(MS_PER_YEAR / state.observedBlockMs);
}

function updateObservedBlockTiming(height, timeMs) {
  const nextHeight = Number(height);
  const nextTime = Number(timeMs);
  const prevHeight = Number(state.latestBlockHeight || 0);
  const prevTime = Number(state.latestBlockTimeMs || 0);
  if (Number.isFinite(prevHeight) && prevHeight > 0 && nextHeight > prevHeight && Number.isFinite(prevTime) && prevTime > 0 && nextTime > prevTime) {
    applyObservedBlockMs((nextTime - prevTime) / (nextHeight - prevHeight));
  }
  if (Number.isFinite(nextTime) && nextTime > 0) {
    state.latestBlockTimeMs = nextTime;
  }
  if (Number.isFinite(nextHeight) && nextHeight > 0) {
    state.latestBlockSeenAtMs = Date.now();
  }
}

async function calibrateObservedBlockTiming() {
  const latest = await api("/thornado/block");
  const latestHeader = latest?.header || latest?.block?.header || {};
  const latestHeight = Number(latestHeader.height || latest?.id?.height || 0);
  const latestTime = Date.parse(latestHeader.time || "");
  if (!Number.isFinite(latestHeight) || latestHeight <= 1 || !Number.isFinite(latestTime)) return;
  const sampleHeight = Math.max(1, latestHeight - 50);
  const sample = await api(`/cosmos/base/tendermint/v1beta1/blocks/${sampleHeight}`);
  const sampleHeader = sample?.block?.header || {};
  const oldHeight = Number(sampleHeader.height || sampleHeight);
  const oldTime = Date.parse(sampleHeader.time || "");
  if (!Number.isFinite(oldHeight) || oldHeight >= latestHeight || !Number.isFinite(oldTime) || latestTime <= oldTime) return;
  applyObservedBlockMs((latestTime - oldTime) / (latestHeight - oldHeight));
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

function mergeReceipts(existing = null, incoming = null) {
  if (!existing?.notes?.length) {
    return incoming || existing;
  }
  if (!incoming?.notes?.length) {
    return existing;
  }
  const notes = [];
  const seen = new Set();
  for (const note of [...existing.notes, ...incoming.notes]) {
    const key = noteKey(note);
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    notes.push(note);
  }
  return {
    ...existing,
    ...incoming,
    notes,
    remainder_sats: Number(existing.remainder_sats || 0) + Number(incoming.remainder_sats || 0)
  };
}

function upsertBatch(batch) {
  const key = batchKey(batch);
  const existingIndex = state.batches.findIndex((item) => batchKey(item) === key);
  if (existingIndex >= 0) {
    const existing = state.batches[existingIndex];
    const keepRecovered = existing.status === "committed" && existing.receipt?.notes?.length && batch.status !== "committed";
    const timing = batchTimingFromSession(batch.session || existing.session, batch.deposit || existing.deposit);
    const depositTxs = Array.isArray(batch.depositTxs)
      ? mergeDepositTxs(batch.depositTxs)
      : mergeDepositTxs(existing.depositTxs);
    state.batches[existingIndex] = {
      ...existing,
      ...batch,
      ...timing,
      issuedAt: batch.issuedAt ?? timing.issuedAt ?? existing.issuedAt,
      expiresAt: batch.expiresAt ?? timing.expiresAt ?? existing.expiresAt,
      amountSats: keepRecovered ? existing.amountSats : batch.amountSats ?? existing.amountSats,
      status: keepRecovered ? existing.status : batch.status ?? existing.status,
      receipt: keepRecovered ? existing.receipt : mergeReceipts(existing.receipt, batch.receipt) ?? existing.receipt,
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

function depositTxErrata(tx = {}) {
  const status = String(tx?.status || tx?.deposit?.status || "").toLowerCase();
  return status === "errata" || status === "reverted" || status === "removed";
}

function depositTxRefunding(tx = {}) {
  const status = String(tx?.status || tx?.deposit?.status || "").toLowerCase();
  return status === "return_queued" || status === "return_complete" || status === "refunded";
}

function depositTxShieldable(tx = {}) {
  return !depositTxErrata(tx) && !depositTxRefunding(tx);
}

function depositTxCount(batch) {
  return mergeDepositTxs(batch?.depositTxs).filter((tx) => !depositTxErrata(tx)).length;
}

function trackedDepositTxCount(batches = []) {
  return batches.reduce((sum, batch) => sum + depositTxCount(batch), 0);
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
  if (status === "errata" || status === "reverted" || status === "removed") {
    return false;
  }
  const stages = txStatus?.stages || {};
  return status === "deposit_matched"
    || status === "committed"
    || stages.inbound_finalised?.completed === true
    || stages.inbound_confirmation_counted?.completed === true;
}

function batchFinalised(batch) {
  if (batch?.status === "committed") {
    return true;
  }
  return finalisedDepositTxs(batch).length > 0;
}

function finalisedDepositTxs(batch) {
  return mergeDepositTxs(batch?.depositTxs)
    .filter((tx) => depositTxShieldable(tx) && depositFinalised({}, tx.deposit, tx.txStatus));
}

function firstFinalisedDepositTx(batch) {
  return finalisedDepositTxs(batch)[0] || null;
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
  const status = String(batch?.status || batch?.deposit?.status || batch?.session?.status || "").toLowerCase();
  if (status === "errata" || status === "reverted" || status === "removed") {
    return "Errata";
  }
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
    <i class="deposit-chevron" aria-hidden="true"></i>
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
        state.stageUserToggled = {};
        state.stageSelectionKey = "";
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
  return findOutboundTxout(payload, inHash, recipient, amountSats)?.item?.out_hash || "";
}

function outboundVout(item = {}) {
  const value = item.out_vout ?? item.outVout ?? item.vout;
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed >= 0 ? parsed : null;
}

function outboundOutpoint(item = {}) {
  const explicit = item.outpoint || item.out_point || item.outPoint;
  if (explicit) return String(explicit);
  const hash = item.out_hash || item.outHash || "";
  const vout = outboundVout(item);
  return hash && vout !== null ? `${hash}:${vout}` : hash;
}

function findOutboundTxout(payload, inHash, recipient = "", amountSats = 0) {
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
      if (matchesInHash || matchesOutput) {
        return { txout, item };
      }
    }
  }
  return null;
}

function pendingPayoutText(payout) {
  if (!payout) {
    return "Pending";
  }
  if (payout.outpoint || payout.outHash) {
    return payout.outpoint || payout.outHash;
  }
  const status = String(payout.status || "").toLowerCase();
  if (status === "pending_batch") {
    const baseMs = msUntilBlockHeight(payout.height) || 0;
    const ms = Math.max(0, baseMs + blocksToMs(1));
    return ms && ms > 0 ? `Pending (${formatPendingDuration(ms)})` : "Pending";
  }
  if (status === "pending_retry") {
    const ms = msUntilBlockHeight(payout.retryUntilHeight);
    return ms && ms > 0 ? `Retrying (${formatPendingDuration(ms)})` : "Retrying";
  }
  if (status === "pending_sign") {
    const estimateMs = blocksToMs(1);
    const elapsedMs = Math.max(0, Date.now() - Number(payout.statusSinceMs || Date.now()));
    const remainingMs = Math.max(0, estimateMs - elapsedMs);
    return remainingMs > 0 ? `Signing (${formatPendingDuration(remainingMs)})` : "Signing";
  }
  return status ? status.replace(/_/g, " ") : "Pending";
}

function pendingPayoutFromTxout(txoutMatch) {
  if (!txoutMatch) return null;
  return {
    height: Number(txoutMatch.txout?.height || 0),
    status: txoutMatch.txout?.status || "",
    outHash: txoutMatch.item?.out_hash || "",
    outVout: outboundVout(txoutMatch.item),
    outpoint: outboundOutpoint(txoutMatch.item),
    retryUntilHeight: Number(txoutMatch.txout?.retry_until_height || 0),
    signingAttempt: Number(txoutMatch.txout?.signing_attempt || 0)
  };
}

function setWithdrawalPayout(key, payout) {
  if (!key || !payout) return;
  const previous = state.withdrawalPayouts[key] || {};
  const status = String(payout.status || "").toLowerCase();
  const previousStatus = String(previous.status || "").toLowerCase();
  state.withdrawalPayouts[key] = {
    ...previous,
    ...payout,
    statusSinceMs: status && status === previousStatus && previous.statusSinceMs
      ? previous.statusSinceMs
      : Date.now()
  };
}

function txoutBatches(payload = {}) {
  if (Array.isArray(payload?.txouts)) {
    return payload.txouts;
  }
  if (Array.isArray(payload?.tx_out)) {
    return payload.tx_out;
  }
  if (payload?.keysign) {
    return [payload.keysign];
  }
  if (payload?.txout) {
    return [payload.txout];
  }
  return [];
}

function cacheRefundTxouts(payload = {}) {
  const byInHash = new Map();
  for (const txout of txoutBatches(payload)) {
    for (const item of txout.tx_array || []) {
      if (String(item.tx_type || "").toLowerCase() !== "refund" || !item.in_hash) {
        continue;
      }
      const inHash = String(item.in_hash).toUpperCase();
      byInHash.set(inHash, {
        ...item,
        height: txout.height || item.height || "",
        status: txout.status || item.status || "",
        signingTransactionId: txout.signing_transaction_id || ""
      });
    }
  }
  state.refundTxByInHash = byInHash;
  return byInHash;
}

async function refreshRefundTxouts() {
  if (state.refundTxoutPending) {
    return;
  }
  state.refundTxoutPending = true;
  try {
    cacheRefundTxouts(await api("/thornado/txout"));
    renderDepositHistory();
  } finally {
    state.refundTxoutPending = false;
  }
}

async function scanKeysignBlocksForOutbound(inHash, recipient = "", amountSats = 0, requestedHeight = 0) {
  const current = await api("/thornado/txout");
  const currentHeight = Number(current?.txout?.height || 0);
  const start = Math.max(1, Number(requestedHeight || 0));
  const foundInQueue = findOutboundTxout(current, inHash, recipient, amountSats);
  if (foundInQueue?.item?.out_hash || !currentHeight || !start) {
    return foundInQueue?.item?.out_hash || "";
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
  const current = await api("/thornado/txout").catch(() => null);
  const currentHeight = Number(current?.txout?.height || 0);
  if (Number.isFinite(currentHeight) && currentHeight > 0) {
    state.latestBlockHeight = currentHeight;
  }
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
    const pending = current ? findOutboundTxout(current, inHash, recipient, amountSats) : null;
    if (pending) {
      setWithdrawalPayout(key, pendingPayoutFromTxout(pending));
    }
    const outHash = pending?.item?.out_hash
      || await scanKeysignBlocksForOutbound(inHash, recipient, amountSats, requestedHeight).catch(() => "");
    state.withdrawnNotes[key] = {
      ...withdrawal,
      withdrawalID,
      inHash,
      outHash,
      outVout: pending?.item ? outboundVout(pending.item) : withdrawal.outVout,
      outpoint: pending?.item ? outboundOutpoint(pending.item) : withdrawal.outpoint,
      recipient,
      amountSats,
      status: outHash ? "spent" : withdrawal.status
    };
  }));
}

function hasPendingWithdrawalPayouts() {
  return Object.values(state.withdrawnNotes || {}).some((withdrawal) => withdrawal && !withdrawal.outHash);
}

async function waitForOutboundHash(inHash, recipient = "", amountSats = 0, requestedHeight = 0, timeoutMs = 900000, onPayout = null) {
  const started = Date.now();
  while (Date.now() - started < timeoutMs) {
    const current = await api("/thornado/txout").catch(() => null);
    const currentHeight = Number(current?.txout?.height || 0);
    if (Number.isFinite(currentHeight) && currentHeight > 0) {
      state.latestBlockHeight = currentHeight;
    }
    const pending = current ? findOutboundTxout(current, inHash, recipient, amountSats) : null;
    if (pending?.item?.out_hash) {
      return pending.item.out_hash;
    }
    if (pending && onPayout) {
      onPayout(pendingPayoutFromTxout(pending));
    } else if (requestedHeight && onPayout) {
      onPayout({ status: "pending_batch", height: Number(requestedHeight || 0) });
    }
    const outHash = await scanKeysignBlocksForOutbound(inHash, recipient, amountSats, requestedHeight).catch((error) => {
      log("withdraw/payout-scan", { error: errorText(error) });
      return "";
    });
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

function formatPendingDuration(ms) {
  const totalSeconds = Math.ceil(Math.max(0, Number(ms || 0)) / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}m ${String(seconds).padStart(2, "0")}s`;
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
  calibrateObservedBlockTiming().catch((error) => log("block/timing", { error: errorText(error) }));
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
  $("secretBirthday").hidden = !isRevealed;
  $("secretHelp").hidden = !isRevealed;
  $("secretActions").hidden = !isRevealed;
  $("secretValue").textContent = isRevealed ? secret : "";
  $("secretBirthday").textContent = isRevealed ? `Birthday ${walletBirthdayWord()}` : "";
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
  state.withdrawalPayouts = {};
  state.depositDropdownOpen = false;
  state.openBatchDropdown = "";
  state.noteRecoveryPending = false;
  state.noteRecoveryQueued = false;
  state.noteRecoveryStatus = "idle";
  state.noteRecoveryBatchKey = "";
  state.noteRecoveryProgress = null;
  state.paneBatchKeys = { deposit: "", shield: "", withdraw: "" };
  state.stageUserToggled = {};
  state.stageSelectionKey = "";
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
      const rowStatus = String(tx.deposit?.status || selectedBatch.status || "").toLowerCase();
      const rowErrata = rowStatus === "errata" || rowStatus === "reverted" || rowStatus === "removed";
      const rowRefunding = depositTxRefunding(tx);
      const refundTx = rowRefunding ? state.refundTxByInHash.get(String(tx.txid || "").toUpperCase()) : null;
      const refundHash = String(refundTx?.out_hash || "").trim();
      const refundStatus = `
        <span class="deposit-refund-status">
          <strong>Refunded</strong>
          <small>${refundHash ? txHashLink(refundHash) : "pending signature"}</small>
        </span>
      `;
      detailRows.push(`
        <div class="row${rowErrata ? " deposit-errata" : ""}">
          <span>${txHashLink(tx.txid)}</span>
          <strong>${rowErrata ? `${btcAmount(Number(tx.amountSats || 0))} · Errata` : rowRefunding ? refundStatus : `${btcAmount(Number(tx.amountSats || 0))} · ${confirmationProgressLabel(txProgress)}`}</strong>
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

function commitmentScanVisible() {
  return Boolean(state.noteRecoveryProgress)
    && state.noteRecoveryStatus === "searching_commitments";
}

function nullifierScanVisible() {
  return Boolean(state.noteRecoveryProgress)
    && state.noteRecoveryStatus === "searching_nullifiers";
}

function noteRecoveryVisible() {
  return commitmentScanVisible() || nullifierScanVisible();
}

function noteRecoveryLabel(kind = state.noteRecoveryStatus) {
  if (kind === "searching_commitments") {
    return "Matching notes";
  }
  if (kind === "searching_nullifiers") {
    return "Checking spent status";
  }
  if (state.noteRecoveryStatus === "done") {
    return "Sync complete";
  }
  return "Syncing public set";
}

function commitmentProgress(progress = {}) {
  const loaded = Number(progress.notesLoaded ?? progress.notes ?? progress.loaded ?? 0);
  const rawTotal = Number(progress.noteTotal ?? progress.notesTotal ?? progress.total ?? 0);
  const total = rawTotal > loaded ? rawTotal : 0;
  const percent = total > 0 ? Math.min(100, Math.floor((loaded / total) * 100)) : Number(progress.percent || 0);
  return { ...progress, loaded, total, percent, stream: "notes" };
}

function nullifierProgress(progress = {}) {
  const loaded = Number(progress.nullifiersLoaded ?? progress.nullifiers ?? progress.loaded ?? 0);
  const rawTotal = Number(progress.nullifierTotal ?? progress.nullifiersTotal ?? progress.total ?? 0);
  const total = rawTotal > loaded ? rawTotal : 0;
  const percent = total > 0 ? Math.min(100, Math.floor((loaded / total) * 100)) : Number(progress.percent || 0);
  return { ...progress, loaded, total, percent, stream: "nullifiers" };
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
  if (selectedBatchKey !== state.stageSelectionKey) {
    const hadSelection = Boolean(state.stageSelectionKey);
    state.stageSelectionKey = selectedBatchKey;
    if (hadSelection) {
      state.stageUserToggled = {};
    }
  }
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
    autoSetStageExpanded("stageDeposit", false);
    autoSetStageExpanded("stageDepositTrack", false);
    autoSetStageExpanded("stageShield", true);
    state.shieldStageOpenedForDeposit = selectedBatchKey;
  }
  if (!addressFocusActive && hasMatureNote && selectedMatureNoteKey && state.withdrawStageOpenedForNotes !== selectedMatureNoteKey) {
    autoSetStageExpanded("stageDeposit", false);
    autoSetStageExpanded("stageDepositTrack", false);
    autoSetStageExpanded("stageShield", false);
    autoSetStageExpanded("stageWait", false);
    autoSetStageExpanded("stageWithdraw", true);
    state.withdrawStageOpenedForNotes = selectedMatureNoteKey;
  }
  const hasTrackedDeposits = displayBatches.length > 0;
  const hasUnconfirmedSeenDeposit = displayBatches.some((item) => {
    const progress = batchConfirmationProgress(item);
    return progress.seen && progress.current < progress.required;
  });
  const showDepositTracking = hasDeposit || hasTrackedDeposits || hasUnconfirmedSeenDeposit;
  if (!hasDeposit && !hasTrackedDeposits && !state.depositRequestPending) {
    autoSetStageExpanded("stageDeposit", true);
  }
  if (hasDeposit && !selectedFinalised && !isDepositExpired) {
    autoSetStageExpanded("stageDeposit", true);
    autoSetStageExpanded("stageDepositTrack", true);
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
  const trackedDepositCount = trackedDepositTxCount(displayBatches);
  $("depositTrackSummary").textContent = trackedDepositCount
    ? `${trackedDepositCount} deposit${trackedDepositCount === 1 ? "" : "s"} tracked`
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
  renderEmbeddedNodeFlows();
  renderShieldedSummary();
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
  state.shielderSyncPendingFromHeight = 0;
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
  const fromHeight = Math.max(0, Math.floor(Number(options.fromHeight || 0)));
  const now = Date.now();
  if (!force && state.shielderSyncCache && state.shielderSyncCache.fromHeight === fromHeight && now - state.shielderSyncCache.fetchedAt < POOL_REFRESH_MS) {
    if (onProgress) {
      onProgress({ percent: 100, done: true, ...state.shielderSyncCache.stats });
    }
    return state.shielderSyncCache.payload;
  }
  if (!force && state.shielderSyncPending && state.shielderSyncPendingFromHeight === fromHeight) {
    return state.shielderSyncPending;
  }
  state.shielderSyncPendingFromHeight = fromHeight;
  state.shielderSyncPending = (async () => {
    const payload = { notes: [], nullifiers: [], deposits: [] };
    const cursors = { deposit: "", note: "", nullifier: "" };
    let stats = { loaded: 0, total: 0, percent: 0 };
    let hasMore = true;
    let pages = 0;
    do {
      const params = new URLSearchParams({ limit: String(SHIELDER_SYNC_PAGE_LIMIT) });
      if (fromHeight > 0) params.set("from_height", String(fromHeight));
      if (cursors.deposit) params.set("deposit_cursor", cursors.deposit);
      if (cursors.note) params.set("note_cursor", cursors.note);
      if (cursors.nullifier) params.set("nullifier_cursor", cursors.nullifier);
      const page = await api(`/thornado/shielder/sync?${params.toString()}`);
      pages += 1;
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
      const advertisedTotal = pageTotal(page, "deposit", payload.deposits)
        + pageTotal(page, "note", payload.notes)
        + pageTotal(page, "nullifier", payload.nullifiers);
      const depositTotal = pageTotal(page, "deposit", payload.deposits);
      const noteTotal = pageTotal(page, "note", payload.notes);
      const nullifierTotal = pageTotal(page, "nullifier", payload.nullifiers);
      const loaded = payload.deposits.length + payload.notes.length + payload.nullifiers.length;
      const total = advertisedTotal > loaded ? advertisedTotal : 0;
      stats = {
        phase: "public",
        loaded,
        total,
        percent: total > 0 ? Math.min(99, Math.floor((loaded / total) * 100)) : hasMore ? Math.min(95, 10 + pages * 12) : 100,
        deposits: payload.deposits.length,
        notes: payload.notes.length,
        nullifiers: payload.nullifiers.length,
        fromHeight,
        depositsLoaded: payload.deposits.length,
        notesLoaded: payload.notes.length,
        nullifiersLoaded: payload.nullifiers.length,
        depositTotal,
        noteTotal,
        nullifierTotal
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
      state.shielderSyncCache = { fetchedAt: Date.now(), payload: normalized, stats, fromHeight };
      updatePublicNoteBuckets(normalized);
      return normalized;
  })().finally(() => {
      state.shielderSyncPending = null;
      state.shielderSyncPendingFromHeight = 0;
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
  const blockTime = Date.parse(payload?.header?.time || payload?.block?.header?.time || "");
  const numericHeight = Number(height);
  if (Number.isFinite(numericHeight)) {
    updateObservedBlockTiming(numericHeight, blockTime);
    state.latestBlockHeight = numericHeight;
    if (!Number.isFinite(blockTime)) {
      state.latestBlockSeenAtMs = Date.now();
    }
  }
  state.nodeConnected = true;
  state.nodeStatusText = "connected";
  $("blockHeight").textContent = String(height);
  $("blockProducer").textContent = short(producer, 10, 8);
  $("blockProducer").title = producer;
  renderStatusControls();
  renderDepositExpiry();
  if (hasPendingWithdrawalPayouts()) {
    renderNotes();
  }
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
  const amount = Number(denominationSats || 0);
  if (!Number.isSafeInteger(amount) || amount <= 0) {
    return 0;
  }
  return Math.floor(amount * WITHDRAWAL_FEE_BASIS_POINTS / 10000);
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
const MENU_OPEN_KEY = "thornado-menu-open-v1";

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
  localStorage.setItem(MENU_OPEN_KEY, opening ? "1" : "0");
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
    passphrase: $("walletPassphrase").value || "",
    birthday_ms: Number(state.walletBirthdayMs || 0),
    birthday_height: Number(state.walletBirthdayHeight || 0)
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
    state.walletBirthdayMs = Number(payload.birthday_ms || 0);
    state.walletBirthdayHeight = Number(payload.birthday_height || 0);
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
  setWalletBirthdayNow(true);
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
    item
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
          txhash: spent.withdrawal_id || spent.withdrawalId,
          withdrawalID: spent.withdrawal_id || spent.withdrawalId,
          outHash: spent.out_hash || spent.outHash,
          outVout: outboundVout(spent),
          outpoint: outboundOutpoint(spent),
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
  const statusErrata = status === "errata" || status === "reverted" || status === "removed";
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
  const final = !statusErrata && (status === "deposit_matched"
    || status === "committed"
    || countedStage.completed === true
    || (!hasExplicitProgress && hasDepositRecord && !observedHeight));
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
  if (!batch) {
    return batch;
  }
  const txs = mergeDepositTxs(depositTxs);
  batch.depositTxs = txs;
  if (!txs.length) {
    batch.amountSats = 0;
    batch.inboundTxId = "";
    batch.txStatus = null;
    if (String(batch.deposit?.status || batch.status || "").toLowerCase() !== "errata") {
      batch.depositId = "";
      batch.deposit = null;
      batch.status = batch.depositAddress ? "address_issued" : batch.status;
    }
    return batch;
  }
  const liveTxs = txs.filter((tx) => !depositTxErrata(tx));
  const shieldableTxs = txs.filter((tx) => depositTxShieldable(tx));
  const latest = liveTxs[liveTxs.length - 1] || txs[txs.length - 1];
  batch.inboundTxId = latest?.txid || batch.inboundTxId;
  batch.depositId = latest?.deposit?.deposit_id || latest?.txid || batch.depositId;
  batch.amountSats = liveTxs.reduce((sum, tx) => sum + Number(tx.amountSats || 0), 0);
  batch.deposit = latest?.deposit || batch.deposit;
  batch.txStatus = latest?.txStatus || batch.txStatus;
  if (liveTxs.length && liveTxs.every((tx) => depositTxRefunding(tx))) {
    batch.status = latest?.status || latest?.deposit?.status || batch.status;
  } else if (shieldableTxs.length && shieldableTxs.every((tx) => tx.progress?.current >= tx.progress?.required)) {
    batch.status = batch.status === "committed" ? batch.status : "deposit_matched";
  } else if (shieldableTxs.length) {
    batch.status = batch.status === "committed" ? batch.status : "deposit_observed";
  }
  return batch;
}

async function pruneMissingDepositTxs(sync = null) {
  const publicDepositIds = new Set((sync?.deposits || [])
    .map((deposit) => String(deposit.deposit_id || deposit.txid || deposit.tx_id || "").toUpperCase())
    .filter(Boolean));
  for (const batch of state.batches) {
    const txs = mergeDepositTxs(batch.depositTxs);
    if (!txs.length) {
      continue;
    }
    const kept = [];
    let changed = false;
    for (const tx of txs) {
      const txid = String(tx.txid || "").toUpperCase();
      if (!txid || depositTxErrata(tx) || publicDepositIds.has(txid)) {
        kept.push(tx);
        continue;
      }
      const txStatus = await api(`/thornado/tx/${txid}`).catch(() => null);
      const liveTx = txStatus?.observed_tx?.tx || txStatus?.txs?.[0]?.tx || null;
      const finalised = txStatus?.stages?.inbound_finalised?.completed === true || txStatus?.finalised_height > 0;
      if (liveTx?.id && finalised) {
        kept.push({ ...tx, txStatus });
        continue;
      }
      changed = true;
    }
    if (changed) {
      applyDepositTxAggregate(batch, kept);
    }
  }
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
    function outboundVout(item = {}) {
      const value = item.out_vout ?? item.outVout ?? item.vout;
      const parsed = Number(value);
      return Number.isInteger(parsed) && parsed >= 0 ? parsed : null;
    }
    function outboundOutpoint(item = {}) {
      const explicit = item.outpoint || item.out_point || item.outPoint;
      if (explicit) return String(explicit);
      const hash = item.out_hash || item.outHash || "";
      const vout = outboundVout(item);
      return hash && vout !== null ? \`\${hash}:\${vout}\` : hash;
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
          item
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
            const withdrawal = spentNullifiers.get(nullifierHashKey) || {};
            withdrawnNotes.push({
              key: noteKey(note),
              txhash: withdrawal.withdrawal_id || withdrawal.withdrawalId,
              withdrawalID: withdrawal.withdrawal_id || withdrawal.withdrawalId,
              outHash: withdrawal.out_hash || withdrawal.outHash,
              outVout: outboundVout(withdrawal),
              outpoint: outboundOutpoint(withdrawal),
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
  const knownBatchKeysBeforeScan = new Set(state.batches.map((batch) => batchKey(batch)));
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
  const newlyVisibleBatch = sortedBatches(visibleDepositBatches({ includeExpired: true, includeIssued: true }))
    .find((batch) => !knownBatchKeysBeforeScan.has(batchKey(batch)));
  const preservedBatch = preserveSelection
    ? state.batches.find((batch) => batchKey(batch) === String(selectedKeyBeforeScan))
    : null;
  const openIssuedBatch = [...state.batches].reverse().find((batch) => normalizeDepositType(batch.depositType) === requestedPurpose && batchIssuedUnexpired(batch));
  const latestBatch = state.batches[state.batches.length - 1];
  if (newlyVisibleBatch) {
    state.paneBatchKeys.deposit = batchKey(newlyVisibleBatch);
    activateDepositBatch(newlyVisibleBatch);
    $("stageDepositTrack").dataset.expanded = "1";
  } else if (preservedBatch) {
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
      const recoveryFromHeight = walletBirthdayStartHeight();
      sync = await shielderSync({
        force: true,
        fromHeight: recoveryFromHeight,
        onProgress: (progress) => {
          setNoteRecoveryProgress(commitmentProgress(progress));
        }
      });
      await pruneMissingDepositTxs(sync);
      state.noteRecoveryStatus = "searching_commitments";
      state.noteRecoveryProgress = {
        phase: "local",
        percent: 0,
        loaded: 0,
        total: sync.notes?.length || 0,
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
      state.noteRecoveryStatus = "searching_nullifiers";
      state.noteRecoveryProgress = {
        phase: "local",
        loaded: sync.nullifiers?.length || 0,
        total: sync.nullifiers?.length || 0,
        percent: 100,
        notes: sync.notes?.length || 0,
        nullifiers: sync.nullifiers?.length || 0,
        done: true
      };
      updateDashboard();
      for (const item of knownRecovery.withdrawn || []) {
        state.withdrawnNotes[item.key] = {
          txhash: item.txhash,
          withdrawalID: item.withdrawalID || item.txhash,
          outHash: item.outHash,
          outVout: item.outVout,
          outpoint: item.outpoint,
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
      await pruneMissingDepositTxs(sync);
      hydrateReceipt();
      openWithdrawForRecoveredNotes();
      state.noteRecoveryStatus = "done";
      state.noteRecoveryProgress = {
        ...state.noteRecoveryProgress,
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
  const amountSats = Number(note?.denomination_sats || 0);
  const feeSats = withdrawalFeeForDenomination(amountSats);
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
	const note = state.receipt?.notes?.[noteIndex];
	const contextKey = withdrawalProofContextKey(note, options);
	if (!proof || !publicInputs || state.lastWithdrawalContextKey !== contextKey) {
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

function ownedNoteSnapshot() {
  const buckets = {};
  const commitments = new Set();
  const seen = new Set();
  for (const batch of state.batches || []) {
    for (const note of batch?.receipt?.notes || []) {
      const denomination = Number(note?.denomination_sats || 0);
      if (!denomination) {
        continue;
      }
      const key = noteKey(note);
      if (seen.has(key)) {
        continue;
      }
      seen.add(key);
      if (note?.commitment) {
        commitments.add(String(note.commitment));
      }
      buckets[denomination] = (buckets[denomination] || 0) + 1;
    }
  }
  return { buckets, commitments };
}

function otherNotesInDenomination(denomination, ownedSnapshot) {
  const publicNotes = state.shielderSyncCache?.payload?.notes || [];
  if (publicNotes.length) {
    return publicNotes.filter((note) =>
      Number(note?.denomination_sats || 0) === denomination &&
      !ownedSnapshot.commitments.has(String(note?.commitment || ""))
    ).length;
  }
  const publicBuckets = state.publicNoteBuckets || {};
  return Math.max(0, Number(publicBuckets[denomination] || 0) - Number(ownedSnapshot.buckets[denomination] || 0));
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

function firstProtocolRedeemableShieldedNote() {
  const items = shieldedNoteItems()
    .filter((item) => item.mature && !item.spent)
    .sort((a, b) => Number(b.batch?.displayIndex ?? b.batch?.depositIndex ?? 0) - Number(a.batch?.displayIndex ?? a.batch?.depositIndex ?? 0));
  const item = items[0];
  if (!item?.batch?.receipt?.notes?.length) {
    return null;
  }
  state.receipt = item.batch.receipt;
  state.activeBatchIndex = Number(item.batch.depositIndex || 0);
  state.activeBatchKey = batchKey(item.batch);
  const index = item.batch.receipt.notes.findIndex((note) => noteKey(note) === item.key);
  if (index < 0) {
    return null;
  }
  state.selectedNote = index;
  return { batch: item.batch, note: item.note, index };
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
  const finalisedTxRows = finalisedDepositTxs(selectedBatch);
  const shieldableTxRows = txRows.filter((tx) => depositTxShieldable(tx));
  const fallbackTxid = shieldableTxRows[0]?.txid || (!txRows.length ? selectedBatch.inboundTxId || selectedBatch.depositId || "" : "");

  const appendDepositTxRows = () => {
    const eligibleRows = finalisedTxRows.length ? finalisedTxRows : [];
    if (eligibleRows.length) {
      for (const tx of eligibleRows) {
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

  if (commitmentScanVisible()) {
    if (!notes.length) {
      appendDepositTxRows();
    }
    card.append(renderScanProgressRow(noteRecoveryLabel("searching_commitments")));
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
        const maturityLabel = mature ? "Mature" : `Maturing (${formatWaitClock(batchMaturityMs(selectedBatch))})`;
        const row = document.createElement("div");
        row.className = "batch-note-row";
        row.innerHTML = `<span>${btcAmount(note.denomination_sats)}</span><strong>${spent ? "Withdrawn" : maturityLabel}</strong>`;
        card.append(row);
      }
    }
    const shieldedTxIds = new Set([...groups.keys()].map((txid) => String(txid || "").toUpperCase()));
    for (const tx of finalisedTxRows.filter((row) => !shieldedTxIds.has(String(row.txid || "").toUpperCase()))) {
      const header = document.createElement("div");
      header.className = "shield-summary-row";
      header.innerHTML = `<span>${txHashLink(tx.txid)}</span><strong>${btcAmount(Number(tx.amountSats || 0))}</strong>`;
      card.append(header);
      const actionRow = document.createElement("div");
      actionRow.className = "batch-note-row";
      const button = document.createElement("button");
      button.type = "button";
      button.disabled = state.shieldPending;
      button.innerHTML = state.shieldPending && batchKey(selectedBatch) === String(state.activeBatchKey || "")
        ? '<span class="button-spinner" aria-hidden="true"></span>Shielding'
        : "Shield Deposit";
      button.addEventListener("click", () => run(() => shieldDeposit(batchKey(selectedBatch), tx.txid)));
      actionRow.innerHTML = "<span></span>";
      actionRow.append(button);
      card.append(actionRow);
    }
  } else if (commitmentScanVisible()) {
    // Progress row rendered above the empty state.
  } else if (String(selectedBatch.status || "").toLowerCase() === "committed") {
    appendDepositTxRows();
    const row = document.createElement("div");
    row.className = "batch-note-row";
    row.innerHTML = "<span>Already shielded</span><strong>Searching notes...</strong>";
    card.append(row);
  } else if (batchFinalised(selectedBatch)) {
    appendDepositTxRows();
    if (finalisedTxRows.length || !txRows.length) {
      const actionRow = document.createElement("div");
      actionRow.className = "batch-note-row";
      const button = document.createElement("button");
      button.type = "button";
      button.disabled = state.shieldPending;
      button.innerHTML = state.shieldPending && batchKey(selectedBatch) === String(state.activeBatchKey || "")
        ? '<span class="button-spinner" aria-hidden="true"></span>Shielding'
        : "Shield Deposit";
      button.addEventListener("click", () => run(() => shieldDeposit(batchKey(selectedBatch), finalisedTxRows[0]?.txid || "")));
      actionRow.innerHTML = "<span></span>";
      actionRow.append(button);
      card.append(actionRow);
    } else {
      const row = document.createElement("div");
      row.className = "batch-note-row";
      row.innerHTML = "<span>Waiting for finalised deposit</span><strong>not ready</strong>";
      card.append(row);
    }
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
  if (nullifierScanVisible()) {
    el.append(renderScanProgressRow(noteRecoveryLabel("searching_nullifiers")));
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
  const ownedSnapshot = ownedNoteSnapshot();
  if (!state.shielderSyncCache?.payload?.notes?.length && !Object.keys(state.publicNoteBuckets || {}).length) {
    refreshPublicNoteBuckets().catch((error) => log("pool/sync", { error: errorText(error) }));
  }
  for (const note of selectedBatch.receipt.notes) {
    const denomination = Number(note.denomination_sats || 0);
    const otherNotes = otherNotesInDenomination(denomination, ownedSnapshot);
    const index = globalNoteIndex(note);
    const key = noteKey(note);
    const withdrawal = state.withdrawnNotes[key];
    const payoutOutHash = state.withdrawalPayouts[key]?.outHash || "";
    const effectiveOutHash = withdrawal?.outHash || payoutOutHash;
    const hasOutboundHash = Boolean(effectiveOutHash);
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
    const isPending = isWithdrawing || (isSpent && !hasOutboundHash) || (withdrawal && !hasOutboundHash);
    const isWithdrawn = hasOutboundHash;
    row.innerHTML = `<span>${btcAmount(note.denomination_sats)}</span>`;
    if (isWithdrawn || isPending) {
      const status = document.createElement("strong");
      status.className = "withdraw-status";
    if (!hasOutboundHash && String(state.withdrawalPayouts[key]?.status || "").toLowerCase() === "pending_sign") {
      status.classList.add("signing");
      }
      status.innerHTML = hasOutboundHash
        ? txOutpointLink(effectiveOutHash, withdrawal?.outVout ?? state.withdrawalPayouts[key]?.outVout, withdrawal?.outpoint ?? state.withdrawalPayouts[key]?.outpoint)
        : escapeHtml(pendingPayoutText(state.withdrawalPayouts[key]));
      row.append(status);
    } else {
      const count = document.createElement("span");
      count.className = "pool-inline-count";
      count.textContent = `${otherNotes} other ${otherNotes === 1 ? "note" : "notes"}`;
      action.textContent = feeCovered ? "Withdraw" : "Fee too high";
      action.disabled = !feeCovered;
      action.addEventListener("click", () => {
        state.receipt = selectedBatch.receipt;
        state.activeBatchIndex = Number(selectedBatch.depositIndex || 0);
        state.activeBatchKey = batchKey(selectedBatch);
        state.selectedNote = selectedBatch.receipt.notes.findIndex((item) => noteKey(item) === key);
        openWithdrawAddressModal(Math.max(0, state.selectedNote));
      });
      row.append(count, action);
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
    state.stageUserToggled = {};
    state.stageSelectionKey = "";
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

async function shieldDeposit(targetBatchKey = state.activeBatchKey || "", targetTxId = "") {
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
    const finalisedTxRows = finalisedDepositTxs(batch);
    const targetKey = String(targetTxId || "").toUpperCase();
    const finalisedTx = targetKey
      ? finalisedTxRows.find((tx) => String(tx.txid || "").toUpperCase() === targetKey)
      : finalisedTxRows[0];
    const shieldRef = finalisedTx?.deposit?.deposit_id || finalisedTx?.txid || batch?.depositId || batch?.inboundTxId || "";
    const amountSats = Number(finalisedTx?.amountSats || 0) || Number(batch?.amountSats || 0) || getAmountSats();
    if (!shieldRef) {
      throw new Error("deposit id is not known yet");
    }
    if (mergeDepositTxs(batch?.depositTxs).length && !finalisedTx) {
      throw new Error("deposit is not finalised yet");
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
    const requestedHeight = Number(redeem?.requested_height || 0);
    const netSats = Math.max(0, Number(note.denomination_sats || 0) - withdrawalFeeForDenomination(note.denomination_sats));
    state.withdrawnNotes[key] = { txhash: payload.txhash, withdrawalID, inHash, status: "pending" };
    setWithdrawalPayout(key, { status: "pending_batch", height: requestedHeight });
    renderNotes();
    const outHash = await waitForOutboundHash(
      inHash,
      $("recipient").value.trim(),
      netSats,
      requestedHeight,
      900000,
      (payout) => {
        if (payout) {
          setWithdrawalPayout(key, payout);
          renderNotes();
        }
      }
    );
    const payout = state.withdrawalPayouts[key] || {};
    state.withdrawnNotes[key] = {
      txhash: payload.txhash,
      withdrawalID,
      inHash,
      outHash,
      outVout: payout.outVout,
      outpoint: payout.outpoint
    };
    if (outHash) {
      delete state.withdrawalPayouts[key];
    }
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

function setWithdrawAddressMode(mode = "withdraw") {
  state.withdrawAddressMode = mode;
  const isBid = mode === "bid";
  if ($("withdrawAddressTitle")) $("withdrawAddressTitle").textContent = isBid ? "Bid" : "Withdraw";
  if ($("withdrawRecipientLabel")) $("withdrawRecipientLabel").textContent = isBid ? "Node address bidding to" : "Receive address";
  if ($("withdrawRecipientInput")) $("withdrawRecipientInput").placeholder = isBid ? "Node address" : "New BTC receive address";
  if ($("confirmWithdrawAddress")) $("confirmWithdrawAddress").textContent = isBid ? "Bid" : "Withdraw";
}

function openWithdrawAddressModal(noteIndex, mode = "withdraw") {
  state.pendingWithdrawNote = noteIndex;
  setWithdrawAddressMode(mode);
  const input = $("withdrawRecipientInput");
  input.value = "";
  updateWithdrawAddressModal();
  $("withdrawAddressModal").hidden = false;
  setTimeout(() => input.focus(), 0);
}

function closeWithdrawAddressModal() {
  $("withdrawAddressModal").hidden = true;
  state.pendingWithdrawNote = null;
  setWithdrawAddressMode("withdraw");
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
    throw new Error(state.withdrawAddressMode === "bid" ? "node address is required" : "receive address is required");
  }
  const noteIndex = Number(state.pendingWithdrawNote || 0);
  if (state.withdrawAddressMode === "bid") {
    closeWithdrawAddressModal();
    await submitNodeSalesBid(address, noteIndex);
    return;
  }
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
  }
  updateNodeSecretStatus();
  renderNodeSalesList();
  renderNodeSellFlow();
	      log("node/tools/refresh", { metrics, auctions, height });
	    }

function setExplorerText(id, value) {
  const el = $(id);
  if (el) {
    el.textContent = value === undefined || value === null || value === "" ? "unavailable" : String(value);
  }
}

async function apiFirst(paths) {
  let lastError = null;
  for (const path of paths) {
    try {
      return await api(path);
    } catch (error) {
      lastError = error;
    }
  }
  throw lastError || new Error("query unavailable");
}

function numberValue(value) {
  const parsed = Number(value || 0);
  return Number.isFinite(parsed) ? parsed : 0;
}

function intLabel(value) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed.toLocaleString() : "unavailable";
}

function btcLabelFromSats(sats) {
  const parsed = Number(sats || 0);
  return Number.isFinite(parsed) ? btcAmount(parsed) : "unavailable";
}

function coinAmountSats(coin) {
  const asset = String(coin?.asset || coin?.Asset || "").toUpperCase();
  if (asset && !asset.includes("BTC")) {
    return 0;
  }
  return numberValue(coin?.amount ?? coin?.Amount ?? coin?.sats ?? coin?.Sats);
}

function vaultsFromPayload(payload) {
  return Array.isArray(payload?.base_vaults) ? payload.base_vaults
    : Array.isArray(payload?.baseVaults) ? payload.baseVaults
      : Array.isArray(payload?.vaults) ? payload.vaults
        : Array.isArray(payload) ? payload
          : [];
}

function nodesFromPayload(payload) {
  return Array.isArray(payload?.nodes) ? payload.nodes : Array.isArray(payload) ? payload : [];
}

function shortHash(value) {
  return value ? short(String(value), 14, 10) : "unavailable";
}

function renderExplorerRows(id, rows, emptyText) {
  const root = $(id);
  if (!root) return;
  root.textContent = "";
  if (!rows.length) {
    const empty = document.createElement("div");
    empty.className = "pane-empty";
    empty.textContent = emptyText;
    root.append(empty);
    return;
  }
  for (const row of rows) {
    const item = document.createElement("div");
    item.className = "explorer-row";
    item.innerHTML = row;
    root.append(item);
  }
}

function closeExplorerDetail() {
  const modal = $("explorerDetailModal");
  if (modal) {
    modal.hidden = true;
  }
  const body = $("explorerDetailBody");
  if (body) {
    body.textContent = "";
  }
}

function openExplorerDetail(kind, subject, rows, raw = null) {
  const modal = $("explorerDetailModal");
  const title = $("explorerDetailTitle");
  const subtitle = $("explorerDetailSubtitle");
  const body = $("explorerDetailBody");
  if (!modal || !title || !subtitle || !body) {
    return;
  }
  title.textContent = `${kind} Explorer`;
  subtitle.textContent = subject || "";
  body.textContent = "";
  const summary = document.createElement("div");
  summary.className = "panel-list explorer-list";
  for (const [label, value] of rows) {
    const row = document.createElement("div");
    row.className = "row";
    row.innerHTML = `<span>${escapeHtml(label)}</span><strong>${escapeHtml(value === undefined || value === null || value === "" ? "unavailable" : String(value))}</strong>`;
    summary.append(row);
  }
  body.append(summary);
  const pre = document.createElement("pre");
  pre.className = "explorer-detail-raw";
  pre.textContent = JSON.stringify(raw || {}, null, 2);
  body.append(pre);
  modal.hidden = false;
}

function setExplorerResult(id, rows, raw = null, detail = null) {
  const root = $(id);
  if (!root) return;
  root.textContent = "";
  if (!rows.length) {
    const empty = document.createElement("div");
    empty.className = "pane-empty";
    empty.textContent = "No result";
    root.append(empty);
    return;
  }
  const list = document.createElement("div");
  list.className = "panel-list explorer-list";
  for (const [label, value] of rows) {
    const row = document.createElement("div");
    row.className = "row";
    row.innerHTML = `<span>${escapeHtml(label)}</span><strong>${escapeHtml(value === undefined || value === null || value === "" ? "unavailable" : String(value))}</strong>`;
    list.append(row);
  }
  root.append(list);
  if (detail) {
    const actions = document.createElement("div");
    actions.className = "actions explorer-result-actions";
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = "Details";
    button.addEventListener("click", () => openExplorerDetail(detail.kind, detail.subject, rows, raw));
    actions.append(button);
    root.append(actions);
  }
  if (raw) {
    const details = document.createElement("details");
    details.className = "explorer-raw";
    details.innerHTML = `<summary>Raw</summary><pre>${escapeHtml(JSON.stringify(raw, null, 2))}</pre>`;
    root.append(details);
  }
}

function txHashFromResponse(tx) {
  return tx?.observed_tx?.tx?.id
    || tx?.observedTx?.tx?.id
    || tx?.tx_response?.txhash
    || tx?.txResponse?.txhash
    || tx?.hash
    || "";
}

function looksLikeTxHash(query) {
  return /^[0-9a-f]{64}$/i.test(query);
}

function looksLikeBtcAddress(query) {
  return /^(bc1|tb1|bcrt1|[13mn2])[a-z0-9]+$/i.test(query);
}

async function renderNetworkAddressDetail(query, preferNode = false) {
  const [accountResult, depositResult, nodeResult] = await Promise.allSettled([
    api(`/auth/accounts/${encodeURIComponent(query)}`),
    api(`/thornado/deposit/address/txs?address=${encodeURIComponent(query)}`),
    api(`/thornado/node/${encodeURIComponent(query)}`)
  ]);
  const account = accountResult.status === "fulfilled" ? accountResult.value : null;
  const deposits = depositResult.status === "fulfilled" ? depositResult.value : null;
  const node = nodeResult.status === "fulfilled" ? nodeResult.value : null;
  const txs = deposits?.txs || [];
  const isNode = Boolean(node?.node_address || node?.nodeAddress || node?.status || node?.total_bond || node?.totalBond);
  if (preferNode && isNode) {
    const preflight = node?.preflight_status || node?.preflightStatus || {};
    const rows = [
      ["Node", shortHash(node.node_address || node.nodeAddress || query)],
      ["Status", node.status],
      ["Bond", btcLabelFromSats(node.total_bond ?? node.totalBond)],
      ["Version", node.version],
      ["Operator", shortHash(node.node_operator_address || node.nodeOperatorAddress)],
      ["IP", node.ip_address || node.ipAddress],
      ["Preflight", preflight.status],
      ["Missing blocks", intLabel(node.missing_blocks ?? node.missingBlocks)]
    ];
    setExplorerResult("networkExploreResult", rows, node, { kind: "Node", subject: query });
    openExplorerDetail("Node", query, rows, node);
    return;
  }
  const rows = [
    ["Address", shortHash(query)],
    ["Account number", account?.account_number ?? account?.accountNumber],
    ["Sequence", account?.sequence],
    ["Deposit txs", intLabel(txs.length)],
    ["Node status", node?.status || node?.node?.status],
    ["Node bond", isNode ? btcLabelFromSats(node.total_bond ?? node.totalBond ?? node.node?.total_bond ?? node.node?.totalBond) : ""]
  ];
  const kind = looksLikeBtcAddress(query) ? "BTC Address" : "Address";
  const raw = { account, deposits, node };
  setExplorerResult("networkExploreResult", rows, raw, { kind, subject: query });
  openExplorerDetail(kind, query, rows, raw);
}

async function renderNetworkTxDetail(query) {
  const tx = await apiFirst([
    `/thornado/tx/${encodeURIComponent(query)}`,
    `/cosmos/tx/v1beta1/txs/${encodeURIComponent(query)}`
  ]);
  const stages = tx?.stages || {};
  const observed = tx?.observed_tx || tx?.observedTx || {};
  const rows = [
    ["Tx", shortHash(txHashFromResponse(tx) || query)],
    ["Consensus height", intLabel(tx.consensus_height ?? tx.consensusHeight)],
    ["Finalised height", intLabel(tx.finalised_height ?? tx.finalisedHeight)],
    ["Outbound height", intLabel(tx.outbound_height ?? tx.outboundHeight)],
    ["Observed", observed.status || (observed.tx ? "yes" : "")],
    ["Actions", intLabel((tx.actions || []).length)],
    ["Out txs", intLabel((tx.out_txs || tx.outTxs || []).length)],
    ["Stage", stages.status || stages.current || (tx?.tx_response?.code === 0 ? "committed" : "")]
  ];
  setExplorerResult("networkExploreResult", rows, tx, { kind: "Transaction", subject: query });
  openExplorerDetail("Transaction", query, rows, tx);
}

async function searchNetworkExplore() {
  const query = $("networkExploreQuery").value.trim();
  if (!query) {
    throw new Error("search value is required");
  }
  setExplorerResult("networkExploreResult", [["Status", "loading"]]);
  try {
    if (looksLikeTxHash(query)) {
      await renderNetworkTxDetail(query);
      return;
    }
    if (looksLikeBtcAddress(query)) {
      await renderNetworkAddressDetail(query, false);
      return;
    }
    if (/^tthor/i.test(query)) {
      await renderNetworkAddressDetail(query, true);
      return;
    }
    await renderNetworkTxDetail(query);
  } catch (error) {
    try {
      await renderNetworkAddressDetail(query, true);
    } catch {
      setExplorerResult("networkExploreResult", [["Error", errorText(error)]]);
    }
  }
}

function bindExplorerSearch(inputId, buttonId, handler) {
  const input = $(inputId);
  const button = $(buttonId);
  if (button) {
    button.addEventListener("click", () => run(handler));
  }
  if (input) {
    input.addEventListener("keydown", (event) => {
      if (event.key === "Enter") {
        event.preventDefault();
        run(handler);
      }
    });
  }
}

function configValue(configs, key) {
  if (!configs) return 0;
  const normalize = (value) => String(value || "").replace(/[^a-z0-9]/gi, "").toLowerCase();
  const normalizedKey = normalize(key);
  const directKey = Object.keys(configs).find((candidate) => normalize(candidate) === normalizedKey);
  const valuesKey = configs.values && !Array.isArray(configs.values)
    ? Object.keys(configs.values).find((candidate) => normalize(candidate) === normalizedKey)
    : "";
  const configKey = configs.config && !Array.isArray(configs.config)
    ? Object.keys(configs.config).find((candidate) => normalize(candidate) === normalizedKey)
    : "";
  const direct = configs[key]
    ?? (directKey ? configs[directKey] : undefined)
    ?? configs.values?.[key]
    ?? (valuesKey ? configs.values[valuesKey] : undefined)
    ?? configs.config?.[key]
    ?? (configKey ? configs.config[configKey] : undefined);
  if (direct !== undefined) return numberValue(direct?.value ?? direct?.int64_value ?? direct?.int64Value ?? direct);
  const list = Array.isArray(configs?.values) ? configs.values : Array.isArray(configs) ? configs : [];
  const found = list.find((item) => normalize(item?.key) === normalizedKey || normalize(item?.name) === normalizedKey);
  return numberValue(found?.value ?? found?.int64_value ?? found?.int64Value);
}

async function refreshNetworkExplorer() {
  const results = await Promise.allSettled([
    api("/thornado/block"),
    api("/thornado/lastblock"),
    api("/thornado/nodes"),
    apiFirst(["/thornado/nodes/metrics", "/thornado/node/metrics"]),
    api("/thornado/vaults/base"),
    api("/thornado/vaults/solvency"),
    api("/thornado/fees"),
    api("/thornado/shielder/sync?limit=2000"),
    api("/thornado/node/auctions"),
    apiFirst(["/thornado/config", "/thornado/config/nodes"]).catch(() => null)
  ]);
  const value = (index) => results[index].status === "fulfilled" ? results[index].value : null;
  const block = value(0);
  const lastBlocks = value(1);
  const nodes = nodesFromPayload(value(2));
  const metrics = value(3);
  const vaults = vaultsFromPayload(value(4));
  const solvency = value(5);
  const fees = value(6);
  const sync = value(7);
  const auctions = Array.isArray(value(8)?.auctions) ? value(8).auctions : [];
  const configs = value(9);

  const header = block?.header || block?.block?.header || {};
  const blockId = block?.id || block?.block_id || block?.blockId || {};
  setExplorerText("networkExplorerHeight", intLabel(header.height));
  setExplorerText("networkChainId", header.chain_id || header.chainId);
  setExplorerText("networkBlockHash", shortHash(blockId.hash));
  setExplorerText("networkBlockTime", header.time ? new Date(header.time).toLocaleString() : "unavailable");
  setExplorerText("networkTxCount", intLabel((block?.txs || []).length));

  const btcLast = (lastBlocks?.last_blocks || lastBlocks?.lastBlocks || []).find((item) => String(item.chain || "").toUpperCase() === "BTC");
  setExplorerText("networkBtcObserved", btcLast ? intLabel(btcLast.last_observed_in ?? btcLast.lastObservedIn) : "not reported");
  setExplorerText("networkBtcSigned", btcLast ? intLabel(btcLast.last_signed_out ?? btcLast.lastSignedOut) : "not reported");

  const vaultSats = vaults.reduce((sum, vault) => sum + (vault.coins || []).reduce((total, coin) => total + coinAmountSats(coin), 0), 0);
  const inboundCount = vaults.reduce((sum, vault) => sum + numberValue(vault.inbound_tx_count ?? vault.inboundTxCount), 0);
  const outboundCount = vaults.reduce((sum, vault) => sum + numberValue(vault.outbound_tx_count ?? vault.outboundTxCount), 0);
  const pendingCount = vaults.reduce((sum, vault) => sum + ((vault.pending_tx_block_heights || vault.pendingTxBlockHeights || []).length), 0);
  setExplorerText("networkVaultBtc", btcLabelFromSats(vaultSats));
  setExplorerText("networkVaultCount", intLabel(vaults.length));
  setExplorerText("networkVaultInbounds", intLabel(inboundCount));
  setExplorerText("networkVaultOutbounds", intLabel(outboundCount));
  setExplorerText("networkVaultPending", intLabel(pendingCount));
  const solvencyAssets = solvency?.assets || [];
  const btcSolvency = solvencyAssets.find((item) => String(item.asset || "").toUpperCase().includes("BTC"));
  setExplorerText("networkSolvency", btcSolvency ? btcLabelFromSats(btcSolvency.amount) : "unavailable");
  renderExplorerRows("networkVaultsList", vaults.slice(0, 8).map((vault) => `
    <span>${escapeHtml(shortHash(vault.pub_key || vault.pubKey))}</span>
    <strong>${escapeHtml(vault.status || "unknown")}</strong>
    <em>${btcLabelFromSats((vault.coins || []).reduce((sum, coin) => sum + coinAmountSats(coin), 0))}</em>
  `), "No base vaults");

  const notes = sync?.notes || [];
  const nullifiers = sync?.nullifiers || [];
  const noteSats = notes.reduce((sum, note) => sum + numberValue(note.denomination_sats ?? note.denominationSats), 0);
  const spentSats = nullifiers.reduce((sum, item) => sum + numberValue(item.amount_sats ?? item.amountSats), 0);
  setExplorerText("networkShieldedBtc", btcLabelFromSats(Math.max(0, noteSats - spentSats)));
  setExplorerText("networkDepositCount", intLabel(sync?.total_deposits ?? sync?.totalDeposits));
  setExplorerText("networkNoteCount", intLabel(sync?.total_notes ?? sync?.totalNotes));
  setExplorerText("networkNullifierCount", intLabel(sync?.total_nullifiers ?? sync?.totalNullifiers));
  setExplorerText("networkFeeBucket", btcLabelFromSats(fees?.pending_sats ?? fees?.pendingSats));

  const activeNodes = nodes.filter((node) => String(node.status || "").toLowerCase() === "active");
  const standbyNodes = nodes.filter((node) => String(node.status || "").toLowerCase() === "standby");
  const totalBondSats = nodes.reduce((sum, node) => sum + numberValue(node.total_bond ?? node.totalBond ?? node.bond), 0);
  setExplorerText("networkNodeTotal", intLabel(nodes.length));
  setExplorerText("networkActiveNodes", intLabel(metrics?.active_slots ?? metrics?.activeSlots ?? activeNodes.length));
  setExplorerText("networkStandbyNodes", intLabel(metrics?.standby_slots ?? metrics?.standbySlots ?? standbyNodes.length));
  setExplorerText("networkBondedBtc", btcLabelFromSats(metrics?.confirmed_bond_sats ?? metrics?.confirmedBondSats ?? totalBondSats));
  setExplorerText("networkNextSlotBond", btcLabelFromSats(metrics?.next_slot_bond_required_sats ?? metrics?.nextSlotBondRequiredSats));
  setExplorerText("networkAuctionCount", intLabel(auctions.length));
  const churnMinutes = configValue(configs, "Churn_IntervalMinutes");
  setExplorerText("networkChurnCycle", churnMinutes ? `${churnMinutes.toLocaleString()} min cycle` : "unavailable");
  renderExplorerRows("networkNodesList", nodes.slice(0, 10).map((node) => `
    <span>${escapeHtml(shortHash(node.node_address || node.nodeAddress))}</span>
    <strong>${escapeHtml(node.status || "unknown")}</strong>
    <em>${btcLabelFromSats(node.total_bond ?? node.totalBond ?? node.bond)}</em>
  `), "No nodes");

  log("network/explorer", {
    block: Boolean(block),
    nodes: nodes.length,
    vaults: vaults.length,
    sync: Boolean(sync)
  });
}

function nodeStatusValue(node) {
  return node?.status || node?.node_status || node?.active_status || node?.result?.status || node?.result?.node_status || "unknown";
}

function nodeBondValue(node) {
  const value = node?.bond || node?.bond_sats || node?.total_bond || node?.result?.bond || node?.result?.bond_sats || node?.result?.total_bond;
  const numeric = Number(value || 0);
  return numeric > 0 ? btcAmount(numeric) : "unknown";
}

async function refreshNodeLookup() {
  const address = $("nodeLookupAddress")?.value.trim();
  if (!address) {
    $("nodeLookupStatus").textContent = "Enter node address";
    $("nodeLookupBond").textContent = "unknown";
    $("nodeLookupVersion").textContent = "unknown";
    return;
  }
  $("nodeLookupStatus").textContent = "Checking";
  $("nodeLookupBond").textContent = "unknown";
  $("nodeLookupVersion").textContent = "unknown";
  try {
    const payload = await api(`/thornado/node/${encodeURIComponent(address)}`);
    const node = payload?.node || payload?.result || payload;
    $("nodeLookupStatus").textContent = nodeStatusValue(node);
    $("nodeLookupBond").textContent = nodeBondValue(node);
    $("nodeLookupVersion").textContent = node?.version || node?.node_version || node?.result?.version || "unknown";
    log("node/lookup", { address, node });
  } catch (error) {
    $("nodeLookupStatus").textContent = "Not found";
    $("nodeLookupBond").textContent = "unknown";
    $("nodeLookupVersion").textContent = "unknown";
    log("node/lookup", { address, error: errorText(error) });
  }
}

function showNodeWorkflow(workflow) {
  state.nodeWorkflow = workflow || "new";
  if (state.nodeWorkflow === "bond") {
    syncBondNodeAddress();
  }
  document.querySelectorAll("[data-node-section]").forEach((section) => {
    section.hidden = section.dataset.nodeSection !== state.nodeWorkflow;
  });
  document.querySelectorAll("[data-node-subtab]").forEach((button) => {
    button.classList.toggle("active", button.dataset.nodeSubtab === state.nodeWorkflow);
  });
  renderEmbeddedNodeFlows();
  if (state.nodeWorkflow === "income") {
    refreshNodeIncome().catch((error) => log("node/income/refresh", { error: errorText(error) }));
  }
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
  renderNodeBondFlow();
  renderNodeSalesBidFlow();
  renderNodeSellFlow();
}

function syncBondNodeAddress(force = false) {
  const source = $("nodeLookupAddress")?.value.trim() || "";
  const target = $("bondNodePubkey");
  if (!target || !source) {
    return;
  }
  if (force || !target.value.trim()) {
    target.value = source;
  }
}

function renderNodeBondFlow() {
  const root = $("nodeBondFlow");
  if (!root) return;
  syncBondNodeAddress();
  const items = shieldedNoteItems().filter((item) => !item.spent);
  const matureItems = items.filter((item) => item.mature);
  const total = items.reduce((sum, item) => sum + item.denomination, 0);
  const hasShielded = total > 0;
  if ($("nodeBondShieldedBalance")) {
    $("nodeBondShieldedBalance").textContent = btcAmount(total);
  }
  if ($("nodeBondActionSummary")) {
    $("nodeBondActionSummary").textContent = hasShielded
      ? matureItems.length
        ? "Select a node address and bond mature shielded notes."
        : "Shielded notes are still maturing."
      : "No shielded BTC found. Deposit and shield BTC first.";
  }
  if ($("nodeBondAddressLabel")) {
    $("nodeBondAddressLabel").hidden = !hasShielded;
  }
  const noteDetails = $("nodeBondNoteDetails");
  if (noteDetails) {
    noteDetails.textContent = "";
    if (hasShielded) {
      const card = document.createElement("div");
      card.className = "batch-card";
      for (const item of items) {
        const withdrawal = state.withdrawnNotes[item.key];
        const row = document.createElement("div");
        row.className = "batch-note-row";
        row.innerHTML = `<span>${btcAmount(item.denomination)}</span><strong>${withdrawal?.status === "bond" ? "Bonded" : item.mature ? "Ready" : "Maturing"}</strong>`;
        card.append(row);
      }
      noteDetails.append(card);
    }
  }
  if ($("nodeBondDepositShortcut")) {
    $("nodeBondDepositShortcut").hidden = hasShielded;
  }
  if ($("buildBondFromNotesCommand")) {
    $("buildBondFromNotesCommand").hidden = !hasShielded;
    $("buildBondFromNotesCommand").disabled = !matureItems.length;
  }
}

function renderNodeSalesBidFlow() {
  const root = $("nodeSalesBidFlow");
  if (!root) return;
  const items = shieldedNoteItems().filter((item) => !item.spent);
  const matureItems = items.filter((item) => item.mature);
  const total = items.reduce((sum, item) => sum + item.denomination, 0);
  const hasShielded = total > 0;
  if ($("nodeSalesShieldedBalance")) {
    $("nodeSalesShieldedBalance").textContent = btcAmount(total);
  }
  if ($("nodeSalesBidSummary")) {
    $("nodeSalesBidSummary").textContent = hasShielded
      ? matureItems.length
        ? "Select an auction, enter the bidder node address, then bid with mature shielded BTC."
        : "Shielded notes are still maturing."
      : "No shielded BTC found. Deposit and shield BTC first.";
  }
  const noteDetails = $("nodeSalesBidNoteDetails");
  if (noteDetails) {
    noteDetails.textContent = "";
    if (hasShielded) {
      const card = document.createElement("div");
      card.className = "batch-card";
      for (const item of items) {
        const row = document.createElement("div");
        row.className = "batch-note-row";
        row.innerHTML = `<span>${btcAmount(item.denomination)}</span><strong>${item.mature ? "Ready" : "Maturing"}</strong>`;
        card.append(row);
      }
      noteDetails.append(card);
    }
  }
  if ($("nodeSalesDepositShortcut")) {
    $("nodeSalesDepositShortcut").hidden = hasShielded;
  }
  if ($("openNodeSalesBid")) {
    $("openNodeSalesBid").hidden = !hasShielded;
    $("openNodeSalesBid").disabled = !matureItems.length;
  }
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
      renderNodeSalesBidFlow();
      $("openNodeSalesBid")?.scrollIntoView({ behavior: "smooth", block: "center" });
    });
    action.append(button);
    card.append(action);
    el.append(card);
  }
}

function auctionBids(auction) {
  const raw = auction?.bids || auction?.Bids || auction?.bid || auction?.offers || [];
  return Array.isArray(raw) ? raw : [];
}

function bidID(bid) {
  return String(bid?.bid_id || bid?.bidId || bid?.id || bid?.offer_id || bid?.offerId || "");
}

function bidAmountSats(bid) {
  return Number(bid?.amount_sats || bid?.amountSats || bid?.bid_amount_sats || bid?.bidAmountSats || bid?.amount || 0);
}

function estimateHeightFromDateInput(value) {
  const raw = String(value || "").trim();
  const targetMs = /^\d{4}-\d{2}-\d{2}$/.test(raw)
    ? new Date(`${raw}T23:59:59`).getTime()
    : Date.parse(raw);
  if (!Number.isFinite(targetMs)) return "";
  const current = interpolatedBlockHeight();
  const blockMs = Number(state.observedBlockMs || blocksToMs(1));
  if (!Number.isFinite(current) || !Number.isFinite(blockMs) || blockMs <= 0) return "";
  return String(Math.max(1, Math.ceil(current + Math.max(0, targetMs - Date.now()) / blockMs)));
}

async function renderNodeSellFlow() {
  if (!$("saleSellerNodeDisplay")) return;
  let identity = null;
  try {
    identity = await deriveNodeIdentity();
  } catch {
    // Secret is optional while browsing the operator UI.
  }
  const sellerKeys = new Set([identity?.authPubkey, identity?.authAddress].filter(Boolean));
  if ($("saleSellerNodeDisplay")) {
    $("saleSellerNodeDisplay").textContent = identity?.authAddress ? short(identity.authAddress, 18, 10) : "connect secret";
  }
  if ($("saleSellerNodePubkey")) {
    $("saleSellerNodePubkey").value = identity?.authPubkey || "";
  }

  const rows = [];
  for (const auction of state.nodeSales || []) {
    const seller = auction.seller_node_pub_key || auction.sellerNodePubKey || auction.node_pub_key || auction.nodePubKey || "";
    if (sellerKeys.size && seller && !sellerKeys.has(seller)) continue;
    const id = auctionID(auction);
    for (const bid of auctionBids(auction)) {
      rows.push({ auctionID: id, bid, amount: bidAmountSats(bid) });
    }
  }

  const list = $("nodeSellBidsList");
  if (!list) return;
  list.textContent = "";
  if (!rows.length) {
    const empty = document.createElement("div");
    empty.className = "pane-empty";
    empty.textContent = "No bids yet";
    list.append(empty);
    return;
  }
  for (const row of rows) {
    const id = bidID(row.bid);
    const el = document.createElement("div");
    el.className = "note-row";
    el.innerHTML = `
      <span>${escapeHtml(short(id || row.auctionID || "bid", 10, 8))}</span>
      <span>${row.amount ? btcAmount(row.amount) : "unknown"}</span>
    `;
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = "Accept";
    button.addEventListener("click", () => {
      if ($("sellAuctionId")) $("sellAuctionId").value = row.auctionID;
      if ($("sellBidId")) $("sellBidId").value = id;
      run(buildAuctionSelectCommand);
    });
    el.append(button);
    list.append(el);
  }
}

async function buildBondFromNotesCommand() {
  const operatorPubKey = (await deriveNodeIdentity()).authPubkey;
  const nodePubKey = $("bondNodePubkey").value.trim();
  if (!operatorPubKey || !nodePubKey) {
    throw new Error("node address and operator pubkey are required");
  }
  const candidate = firstProtocolRedeemableShieldedNote();
  if (!candidate) {
    throw new Error("shield a BTC deposit and wait until its notes are mature before bonding");
  }
  setMessage("Generating bond proof from shielded note...", "", 8, 120000);
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

function feeBucketBalance(pool) {
  const pending = Number(pool?.pending_sats ?? pool?.pendingSats ?? NaN);
  if (Number.isFinite(pending)) {
    return pending;
  }
  const collected = Number(pool?.total_collected_sats ?? pool?.totalCollectedSats ?? 0);
  const claimed = Number(pool?.total_claimed_sats ?? pool?.totalClaimedSats ?? 0);
  return Math.max(0, collected - claimed);
}

function nodeConsPubKeyValue(node) {
  return node?.node_cons_pub_key || node?.nodeConsPubKey || node?.result?.node_cons_pub_key || node?.result?.nodeConsPubKey || "";
}

async function resolveIncomeNodePubKey() {
  const existing = $("incomeNodePubkey")?.value.trim();
  if (existing) {
    return existing;
  }
  const direct = $("nodeConsPubkey")?.value.trim();
  if (direct) {
    $("incomeNodePubkey").value = direct;
    return direct;
  }
  const selected = $("nodeLookupAddress")?.value.trim() || $("bondNodePubkey")?.value.trim();
  if (!selected) {
    return "";
  }
  if (/pub/i.test(selected)) {
    $("incomeNodePubkey").value = selected;
    return selected;
  }
  const payload = await api(`/thornado/node/${encodeURIComponent(selected)}`);
  const node = payload?.node || payload?.result || payload;
  const nodePubKey = nodeConsPubKeyValue(node);
  if (nodePubKey) {
    $("incomeNodePubkey").value = nodePubKey;
  }
  return nodePubKey;
}

async function refreshNodeIncome() {
  const pool = await api("/thornado/fees").catch(() => null);
  if ($("nodeIncomeFeeBucket")) {
    const bucket = pool ? feeBucketBalance(pool) : NaN;
    $("nodeIncomeFeeBucket").textContent = Number.isFinite(bucket) ? btcAmount(bucket) : "unknown";
  }
  let entitlement = null;
  const nodePubKey = await resolveIncomeNodePubKey().catch(() => "");
  if (nodePubKey) {
    entitlement = await api(`/thornado/fee/entitlement/${encodeURIComponent(nodePubKey)}`).catch(() => null);
  }
  const claimable = Number(entitlement?.claimable_sats || entitlement?.claimableSats || 0);
  if ($("nodeIncomeClaimable")) {
    $("nodeIncomeClaimable").textContent = entitlement ? btcAmount(claimable) : "unknown";
  }
  if ($("nodeIncomeBuildFee")) {
    $("nodeIncomeBuildFee").disabled = !entitlement || claimable <= 0;
  }
  return { pool, entitlement, nodePubKey, claimable };
}

async function prepareNodeFeeShield() {
  await refreshNodeIncome();
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
  const identity = await deriveNodeIdentity();
  const seller = identity.authPubkey;
  const reserve = $("saleReserveSats").value.trim() || "<reserve-sats>";
  const expiry = estimateHeightFromDateInput($("saleExpiryAt")?.value) || $("saleExpiryHeight").value.trim() || "<expiry-height>";
  if ($("saleSellerNodePubkey")) $("saleSellerNodePubkey").value = seller;
  if ($("saleExpiryHeight")) $("saleExpiryHeight").value = expiry;
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
  const payload = await browserTx("/thornado/browser/node/auction-bid-create", {
    auction_id: auctionID,
    operator_pubkey: operatorPubKey,
    node_pubkey: nodePubKey
  }, "Create node sale bid", writeSalesStatus);
  if (payload.txhash) {
    payload.tx_response = await waitForCommittedTx(payload.txhash, 45000);
  }
  const bidID = extractBidID(payload);
  if (bidID) {
    $("salesBidId").value = bidID;
  }
  return payload;
}

async function fundAuctionBidFromNotes() {
  const bidID = $("salesBidId").value.trim();
  if (!bidID) {
    throw new Error("bid id is required");
  }
  const candidate = firstProtocolRedeemableShieldedNote();
  if (!candidate) {
    throw new Error("shield BTC and wait until its notes are mature before funding the bid");
  }
  setMessage("Generating bid funding proof from shielded note...", "", 8, 120000);
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

function extractBidID(payload) {
  const direct = payload?.bid_id || payload?.bidId || payload?.response?.bid_id || payload?.response?.bidId;
  if (direct) {
    return String(direct);
  }
  const haystack = JSON.stringify(payload || {});
  const match = haystack.match(/"bid[_-]?id"\s*:\s*"([^"]+)"/i) || haystack.match(/bid[_-]?id['"=:\s]+([A-Za-z0-9._:-]+)/i);
  return match ? match[1] : "";
}

async function openNodeSalesBid() {
  const candidate = firstProtocolRedeemableShieldedNote();
  if (!candidate) {
    throw new Error("shield BTC and wait until its notes are mature before bidding");
  }
  state.selectedNote = candidate.index;
  openWithdrawAddressModal(candidate.index, "bid");
}

async function submitNodeSalesBid(nodePubKey, noteIndex) {
  const selectedText = $("salesSelectedAuction").textContent.trim();
  const auctionID = state.selectedSaleAuctionId || (selectedText && selectedText !== "none" ? selectedText : "");
  if (!auctionID) {
    throw new Error("select an auction first");
  }
  $("salesBidNodePubkey").value = nodePubKey;
  $("salesBidOperatorPubkey").value = (await deriveNodeIdentity()).authPubkey;
  state.selectedNote = Number(noteIndex || 0);
  const bidPayload = await buildAuctionBidCommand();
  const bidID = $("salesBidId").value.trim() || extractBidID(bidPayload);
  if (!bidID) {
    throw new Error("bid was created, but bid id was not returned");
  }
  $("salesBidId").value = bidID;
  await fundAuctionBidFromNotes();
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

	    async function buildConfigVoteCommand() {
	      const key = $("configVoteKey").value.trim();
	      const value = $("configVoteValue").value.trim();
	      if (!key || value === "") {
	        throw new Error("config and value are required");
	      }
	      await browserTx("/thornado/browser/config/vote", { key, value }, "Vote config");
	    }

	    async function buildProposeUpgradeCommand() {
	      const name = $("upgradeName").value.trim();
	      const height = $("upgradeHeight").value.trim();
	      if (!name || !height) {
	        throw new Error("upgrade name and height are required");
	      }
	      const info = "";
	      await browserTx("/thornado/browser/upgrade/propose", { name, height, info }, "Propose upgrade");
	    }

	    async function buildShieldFeesCommand() {
	      const { entitlement, nodePubKey, claimable } = await refreshNodeIncome();
	      if (!entitlement || !nodePubKey) {
	        throw new Error("select a registered node first");
	      }
	      if (claimable <= 0) {
	        throw new Error("no fees claimable");
	      }
	      const seedHex = await walletRootSeedHex();
	      const owner = await nodeSignerAddress();
	      const depositIndex = Number(state.nextDepositIndexByType.user || 0);
	      const accrued = Number(entitlement.accrued_sats || entitlement.accruedSats || claimable);
	      const feePerSlotShare = Number(entitlement.fee_per_slot_share || entitlement.feePerSlotShare || accrued);
	      const claimRef = `fee-claim:${nodePubKey}:${owner}:${accrued}:${feePerSlotShare}:${depositIndex}`;
	      const wasm = await thornadoWasm();
	      if (!wasm.feeClaimAuthorizationForDepositTypeJson) {
	        throw new Error("fee claim helper unavailable");
	      }
	      const authorization = JSON.parse(wasm.feeClaimAuthorizationForDepositTypeJson(
	        seedHex,
	        "node",
	        BigInt(0),
	        "user",
	        BigInt(depositIndex),
	        claimRef,
	        nodePubKey,
	        owner,
	        BigInt(accrued),
	        BigInt(feePerSlotShare),
	        BigInt(claimable)
	      ));
	      const payload = await browserTx("/thornado/browser/node/shield-fees", {
	        node_pubkey: nodePubKey,
	        operator_signature: authorization.operator_signature,
	        commitments: authorization.commitments,
	        fee_note_pubkeys: authorization.fee_note_pubkeys
	      }, "Claim fees");
	      payload.tx_response = await waitForCommittedTx(payload.txhash);
	      const amountSats = Number(payload.amount_sats || claimable);
	      const fingerprint = await rootFingerprint(seedHex);
	      const receipt = authorization.receipt;
	      receipt.notes = (receipt.notes || []).map((note) => ({
	        ...note,
	        deposit_amount_sats: amountSats,
	        deposit_remainder_sats: Number(receipt.remainder_sats || 0),
	        deposit_index: depositIndex,
	        deposit_type: "user",
	        derivation_path: notePath(depositIndex, note.index + 1, "user"),
	        root_fingerprint: fingerprint
	      }));
	      upsertBatch({
	        depositIndex,
	        batchId: `fee:${payload.deposit_id || claimRef}`,
	        owner,
	        depositType: "user",
	        depositId: payload.deposit_id || claimRef,
	        amountSats,
	        status: "committed",
	        receipt,
	        shieldedAt: Date.now(),
	        maturesAt: Date.now() + DEMO_MATURITY_MS
	      });
	      invalidateShielderSyncCache();
	      state.currentTab = "nodes";
	      updateDashboard();
	      await refreshNodeIncome();
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
  const nextSecret = words.join(" ");
  if (nextSecret !== $("walletRoot").value.trim() || !state.walletBirthdayMs) {
    setWalletBirthdayNow(true);
  }
  $("walletRoot").value = nextSecret;
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
  ["routeStatus", "nodeStatus", "shieldedSummary"].forEach((id) => {
    const menu = $(`${id}Menu`);
    const button = $(`${id}Button`);
    if (menu) {
      menu.hidden = true;
    }
    if (button) {
      button.setAttribute("aria-expanded", "false");
    }
  });
  state.shieldedSummaryOpen = false;
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

function toggleShieldedSummary() {
  const willOpen = !$("shieldedSummaryMenu") || $("shieldedSummaryMenu").hidden;
  closeStatusMenus();
  state.shieldedSummaryOpen = willOpen;
  renderShieldedSummary();
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
    if (isNetwork) {
      refreshNetworkExplorer().catch((error) => log("network/explorer", { error: errorText(error) }));
    }
    if (isNodes) {
      showNodeWorkflow(state.nodeWorkflow || "new");
    }
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
$("shieldedSummaryButton").addEventListener("click", (event) => {
  event.stopPropagation();
  toggleShieldedSummary();
});
$("shieldedSummaryWithdraw").addEventListener("click", (event) => {
  event.stopPropagation();
  openTopbarWithdraw();
});
$("shieldedSummaryRevealSecret").addEventListener("click", (event) => {
  event.stopPropagation();
  openTopbarRevealSecret();
});
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
  if (event.key === "Escape" && !$("explorerDetailModal").hidden) closeExplorerDetail();
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
        state.stageUserToggled = {};
        state.stageSelectionKey = "";
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
$("explorerDetailClose").addEventListener("click", closeExplorerDetail);
$("explorerDetailModal").addEventListener("click", (event) => {
  if (event.target === $("explorerDetailModal")) closeExplorerDetail();
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
$("nodeLookupStatusButton").addEventListener("click", () => run(refreshNodeLookup));
$("nodeLookupAddress").addEventListener("keydown", (event) => {
  if (event.key === "Enter") {
    event.preventDefault();
    run(refreshNodeLookup);
  }
});
$("nodeLookupAddress").addEventListener("input", () => {
  syncBondNodeAddress();
  renderEmbeddedNodeFlows();
});
document.querySelectorAll("[data-node-subtab]").forEach((button) => {
  button.addEventListener("click", () => showNodeWorkflow(button.dataset.nodeSubtab));
});
$("prepareNodeBondDeposit").addEventListener("click", () => run(prepareNodeBondDeposit));
$("nodeBondDepositShortcut").addEventListener("click", () => run(prepareUserPurposeDeposit));
$("nodeBondGetAddress").addEventListener("click", () => run(requestNodePurposeDeposit));
$("nodeBondShield").addEventListener("click", () => run(shieldNodePurposeDeposit));
$("buildBondFromNotesCommand").addEventListener("click", () => run(buildBondFromNotesCommand));
	    $("buildSetIpCommand").addEventListener("click", () => run(buildSetIpCommand));
	    $("buildSetVersionCommand").addEventListener("click", () => run(buildSetVersionCommand));
	    $("buildSetKeysCommand").addEventListener("click", () => run(buildSetKeysCommand));
	    $("buildConfigVoteCommand").addEventListener("click", () => run(buildConfigVoteCommand));
	    $("buildProposeUpgradeCommand").addEventListener("click", () => run(buildProposeUpgradeCommand));
$("nodeIncomePrepareFee").addEventListener("click", () => run(prepareNodeFeeShield));
$("nodeIncomeBuildFee").addEventListener("click", () => run(buildShieldFeesCommand));
bindExplorerSearch("networkExploreQuery", "networkExploreSearch", searchNetworkExplore);
$("buildAuctionCreateCommand").addEventListener("click", () => run(buildAuctionCreateCommand));
$("buildAuctionSelectCommand").addEventListener("click", () => run(buildAuctionSelectCommand));
$("nodeSellPreparePayout").addEventListener("click", () => run(prepareNodeSaleShield));
$("nodeSellBuildPayout").addEventListener("click", () => run(submitNodeSaleShield));
$("nodeSalesDepositShortcut").addEventListener("click", () => run(prepareUserPurposeDeposit));
$("openNodeSalesBid").addEventListener("click", () => run(openNodeSalesBid));
$("nodeSalesBidDeposit").addEventListener("click", () => run(prepareNodeSalesBidDeposit));
$("nodeSalesBidGetAddress").addEventListener("click", () => run(requestNodePurposeDeposit));
$("nodeSalesBidShield").addEventListener("click", () => run(shieldNodePurposeDeposit));
$("buildAuctionBidCommand").addEventListener("click", () => run(buildAuctionBidCommand));
$("fundAuctionBidCommand").addEventListener("click", () => run(fundAuctionBidFromNotes));
	    $("buildPauseCommand").addEventListener("click", () => run(buildPauseCommand));
	    $("buildResumeCommand").addEventListener("click", () => run(buildResumeCommand));
	    $("amountSats").addEventListener("input", updateDashboard);
$("walletRoot").addEventListener("input", () => {
  resetDepositState();
  if ($("walletRoot").value.trim() && !state.walletBirthdayMs) {
    setWalletBirthdayNow(true);
  }
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
state.moreOpen = localStorage.getItem(MENU_OPEN_KEY) === "1";
state.moreSettled = state.moreOpen;
state.quoteWriting = false;
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
    setWalletBirthdayNow(true);
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
  if (hasPendingWithdrawalPayouts() && $("withdrawAddressModal").hidden) {
    renderNotes();
  }
  if (state.depositExpiresAt || state.depositExpiresAtHeight) {
    renderDepositExpiry();
  }
  if (hasMaturingNotes || state.depositExpiresAt || state.depositExpiresAtHeight || hasPendingWithdrawalPayouts()) {
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
  if (hasPendingWithdrawalPayouts()) {
    hydrateWithdrawnNotePayouts()
      .then(() => {
        renderNotes();
        updateDashboard();
      })
      .catch((error) => log("withdraw/hydrate", { error: error?.message || String(error) }));
  }
  refreshReceiptPoolPositions()
    .then(() => {
      renderNotes();
      updateDashboard();
    })
    .catch((error) => log("notes/pool", { error: error?.message || String(error) }));
}, POOL_REFRESH_MS);
refreshHash().catch((error) => setMessage(error.message, "error"));
refreshRefundTxouts().catch((error) => log("refund/txout", { error: error?.message || String(error) }));
refreshNodeCount().catch((error) => log("node/count", { error: error?.message || String(error) }));
setInterval(() => {
  refreshHash().catch((error) => log("block/status", { error: error?.message || String(error) }));
}, 5000);
setInterval(() => {
  refreshRefundTxouts().catch((error) => log("refund/txout", { error: error?.message || String(error) }));
}, POOL_REFRESH_MS);
setInterval(() => {
  refreshNodeCount().catch((error) => log("node/count", { error: error?.message || String(error) }));
}, 15000);
