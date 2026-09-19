import express from "express";
import path from "path";
import http from "http";
import https from "https";
import { fileURLToPath } from "url";
import dotenv from "dotenv";
import { GoogleGenAI } from "@google/genai";

dotenv.config();

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const app = express();
const PORT = 3000;

app.use(express.json({ limit: "20mb" }));

// Lazy initialize Gemini client
let geminiClient: GoogleGenAI | null = null;
function getGeminiClient(): GoogleGenAI | null {
  if (!geminiClient && process.env.GEMINI_API_KEY) {
    geminiClient = new GoogleGenAI({
      apiKey: process.env.GEMINI_API_KEY,
      httpOptions: {
        headers: {
          "User-Agent": "aistudio-build",
        },
      },
    });
  }
  return geminiClient;
}

// Health check endpoint
app.get("/api/health", (_req, res) => {
  res.json({
    status: "ok",
    hasGeminiKey: Boolean(process.env.GEMINI_API_KEY),
    time: new Date().toISOString(),
  });
});

// Chat API endpoint
app.post("/api/chat", async (req, res) => {
  try {
    const { message, agent, referencedDocs, history = [] } = req.body;
    if (!message) {
      return res.status(400).json({ error: "Message is required" });
    }

    const ai = getGeminiClient();

    let docContext = "";
    if (referencedDocs && referencedDocs.length > 0) {
      docContext = `\n【当前引用的企业资料库文档内容】：\n` +
        referencedDocs
          .map(
            (doc: { name: string; content?: string; summary?: string }) =>
              `--- 文件名: ${doc.name} ---\n${doc.content || doc.summary || "（该文档已载入知识库索引）"}`
          )
          .join("\n\n");
    }

    const agentRole = agent
      ? `你现在扮演三甲医院药剂科专属AI专家【${agent.name}】。你的专业身份是：${agent.title}。擅长专科领域：${agent.tags?.join("、") || "临床药学与合理用药"}。专家职责与风格：${agent.description}。请以严谨、循证、合规且高度负责的临床药师口吻进行回答，重点突出禁忌症排查、相互作用分析、法律法规SOP准则与患者用药安全，输出结构清晰，使用规范医学与药学排版 Markdown。`
      : `你是WorkBuddy医院药剂科智能协同助手。你的职责是协助临床药师与药剂科同仁完成处方前置审核、配伍禁忌排查、抗菌药物合理应用管理、药品不良反应(ADR)报告与麻精药品精细化质控。回复应专业严谨、条理分明，使用结构化Markdown排版。`;

    const systemInstruction = `${agentRole}\n${docContext}\n注意：回复要实用、循证严谨，关键预警重点加粗提示，支持分点说明，并给出明确的临床药学干预意见。`;

    if (ai) {
      // Use real Gemini API
      try {
        const chatMessages = [
          ...history.slice(-6).map((h: { role: string; content: string }) => ({
            role: h.role === "user" ? "user" : "model",
            parts: [{ text: h.content }],
          })),
          {
            role: "user",
            parts: [{ text: message }],
          },
        ];

        const response = await ai.models.generateContent({
          model: "gemini-3.8-flash",
          contents: chatMessages,
          config: {
            systemInstruction,
            temperature: 0.7,
          },
        });

        return res.json({
          reply: response.text || "我已处理您的请求，请查看上述建议。",
          provider: "gemini",
        });
      } catch (geminiError) {
        console.warn("Gemini API call failed, falling back to simulated engine:", geminiError);
      }
    }

    // High quality intelligent simulated response for offline/preview demo mode
    let simulatedReply = "";
    const lower = message.toLowerCase();

    if (message.includes("处方") || message.includes("配伍") || message.includes("相互作用") || message.includes("他汀") || message.includes("克拉霉素")) {
      simulatedReply = `### 💊 临床药学 · 处方与医嘱合理性审核意见

**【处方前置拦截警示：严重药物相互作用(DDI)级别 - 禁忌/高危】**

1. **相互作用机制分析**：
   - **阿托伐他汀 (40mg qd)** 主要经肝脏细胞色素 **CYP3A4** 酶代谢。
   - **克拉霉素 (500mg bid)** 属于强效 **CYP3A4** 抑制剂，可使阿托伐他汀体内血药浓度峰值(Cmax)和药时曲线下面积(AUC)骤增 **4~10 倍**。
   - **临床危害**：可显著诱发急性横纹肌溶解综合征（严重肌痛、血清肌酸激酶CK > 正常上限10倍、急性肾小管坏死伴急性肾衰竭）。
   - **地高辛 (0.125mg qd)**：克拉霉素亦抑制肠道及肾小管 P-糖蛋白(P-gp)，可显著提高地高辛血清浓度，增加洋地黄中毒风险。

2. **临床药师干预建议**：
   - 🛑 **方案调整建议一（推荐）**：在克拉霉素抗感染疗程中，**暂停使用阿托伐他汀**；或改用不经过 CYP3A4 代谢的降脂药（如**瑞舒伐他汀** 5~10mg qd 或 **匹伐他汀** 2mg qd）。
   - ⚠️ **方案调整建议二**：若必须使用大环内酯类，抗感染药物可评估更换为对 CYP3A4 几乎无抑制作用的**阿奇霉素**（按适应症评估）。
   - 📊 **监测指标**：急查血清肌酸激酶(CK)、肌红蛋白、肝肾功能、电解质及地高辛血药谷浓度。`;
    } else if (message.includes("抗菌") || message.includes("crkp") || message.includes("不动杆菌") || message.includes("耐药") || message.includes("万古霉素")) {
      simulatedReply = `### 🦠 抗微生物药物合理应用(AMS)专项会诊意见

**【病例焦点：重症耐药菌抗感染方案制定与PK/PD优化】**

1. **病原体特点与用药等级判定**：
   - **CRKP / 泛耐药鲍曼不动杆菌** 属于高度耐药革兰阴性杆菌，所用核心药物（多粘菌素B、替加环素）属于**特殊使用级抗菌药物**，已触发药剂科专科会诊机制。

2. **联合抗感染方案制定建议**：
   - **多粘菌素B (Polymyxin B)**：
     - 首剂负荷剂量：**2.0~2.5 mg/kg**（溶于 100~250ml 5%葡萄糖，静滴 1~2小时）。
     - 维持剂量：**1.25~1.5 mg/kg 每 12 小时一次**。重点防范肾毒性与神经肌肉阻滞。
   - **替加环素 (Tigecycline)**：
     - 首剂负荷量 **100mg**，维持剂量 **50mg q12h**（肺部严重感染推荐剂量增至 100mg q12h，静滴时间 ≥ 60分钟）。

3. **PK/PD 目标与用药监测**：
   - 多粘菌素B目标稳态 AUC24/MIC 需控制在 50~100 (mg·h/L) 治疗窗内。
   - 每日监测血肌酐、尿量及电解质，疗程满 72 小时复评降阶梯指征。`;
    } else if (message.includes("不良反应") || message.includes("adr") || message.includes("双硫仑") || message.includes("红人")) {
      simulatedReply = `### ⚠️ 药品不良反应(ADR)调查与因果关系判定报告

**【事件速报：疑似药物严重不良反应紧急处置】**

1. **Naranjo 药物不良反应因果关系评分判定**：
   - 用药与症状发生存在明确先后时间顺序（+2）
   - 符合已知药物作用或双硫仑样反应机制（+2）
   - 停药后症状随乙醇代谢清除而缓解（+1）
   - 无其他明确器质性原发病可完全解释（+2）
   - **总评分：7 分（评定等级：很可能 Probable 关联）**

2. **紧急救治与处置指引**：
   - 立即吸氧，取平卧位，维持循环与气道通畅；
   - 建立静脉通路，静脉推注地塞米松 5~10mg 或氢化可的松 100mg 缓解休克样过敏反应；
   - 补充 5%葡萄糖氯化钠注射液促进乙醛代谢与排泄，静脉注射维生素C及维生素B6。

3. **法规填报提醒**：
   - 该事件已触发国家药品不良反应直报流程，药剂科专管员已协助生成《国家药品不良反应报告表》电子草案，请在 15 个工作日内完成系统终审直报。`;
    } else if (message.includes("麻精") || message.includes("五专") || message.includes("吗啡") || message.includes("芬太尼") || message.includes("残液")) {
      simulatedReply = `### 🔐 医疗机构麻醉药品与一类精神药品“五专”管理SOP

**【执行标准：国家《麻醉药品和精神药品管理条例》合规落地】**

1. **“五专”核心控制节点**：
   - **专人负责**：由经培训考核合格的执业药师双人专管。
   - **专柜加锁**：双人双锁保险柜存放，钥匙与密码分人保管。
   - **专用账册**：账物必须每日批次清点，做到账物相符、批号对应。
   - **专用处方**：一律使用国家统一印制的**淡红色麻醉药品专用处方**。
   - **专册登记**：详细记录患者病历号、姓名、药品批号、发药数量与双人签名。

2. **空安瓿回收与残液销毁闭环**：
   - 临床病区退回使用后的芬太尼/吗啡注射液空安瓿，药房必须**100%按支回收、双人核对**。
   - 遇有镇痛泵配置或未用完残液，必须由**两名专管药师**在专用销毁登记本上同时签字，依感控规范稀释后排入废液管网，记录保存不少于 3 年。`;
    } else if (message.includes("数据分析") || message.includes("可视化") || message.includes("药占比") || message.includes("指标")) {
      simulatedReply = `### 📊 药剂科医疗质量控制核心指标分析

已为您提炼本季度药事管理核心监控大盘数据：

1. **合理用药核心指标**：
   - **药占比（不含中药饮片）**：**24.8%**（达标，符合三甲评审 ≤ 30% 标准）
   - **门急诊处方前置审核拦截率**：**3.42%**（主要拦截配伍禁忌与超频次）
   - **全院抗菌药物使用强度(AUD)**：**37.2 DDDs**（控制在卫健委 40 DDDs 阈值以内）
2. **重点监控药品管理**：
   - 重点监控目录用药金额同比下降 **16.5%**，集采中选品种采购完成率 **112.4%**。
3. **质控改进建议**：
   - 建议在工作台针对外科Ⅰ类切口围手术期预防用药时机（术前0.5~1小时给药）建立自动化红黄牌预警。`;
    } else if (agent) {
      simulatedReply = `您好！我是三甲医院药剂科专科药师**【${agent.name}】**。收到您的临床药学咨询：“${message}”。

针对您关心的**${agent.tags?.join("、") || "临床药学"}**问题，我的循证药学评估如下：
1. **合规性与安全性审查**：基于国家《处方管理办法》及临床用药指导原则，重点防范超剂量、配伍禁忌与药源性风险。
2. **个体化用药调整**：建议结合患者的年龄、肝肾功能指标（如肌酐清除率）、合并用药清单进行个体化剂量微调。
3. **闭环跟进**：您可以随时在【资料库】关联患者的既往医嘱或病历资料，我将实时为您出具精准的临床药学干预备忘！`;
    } else {
      simulatedReply = `收到！针对您的药事协同需求：“**${message}**”，WorkBuddy 药剂科助手已为您启动智能分析：

1. **临床药学意图识别**：已解析您的处方审查与药学质控需求。
2. **知识库联动**：${referencedDocs && referencedDocs.length > 0 ? `已联动您选中的 ${referencedDocs.length} 篇药剂科专业规范与SOP进行循证推演。` : `建议配合内置【资料库】中的处方审核规范与麻精SOP，以获取更详尽的药事干预草案。`}
3. **专家协同支持**：您可在底部「专家」栏随时召唤【处方前审药师】、【抗微生物管理药师】或【不良反应警戒药师】提供精准支持。

需要我为您进一步生成完整的处方审核干预记录单或不良反应报告吗？`;
    }

    return res.json({
      reply: simulatedReply,
      provider: "simulated",
    });
  } catch (error) {
    console.error("Chat error:", error);
    res.status(500).json({ error: "服务器内部错误，请稍后重试" });
  }
});

// ─────────────────────────────────────────────────────────────
// agent-hub 后端代理：/api/v1/* → AGENT_HUB_URL
// 默认指向本地 mock（zerone-agents/mock-server.mjs, :8081）；
// 连真实 agent-hub 时设环境变量，例如：
//   AGENT_HUB_URL=https://console.zerone.life npm run dev
// ─────────────────────────────────────────────────────────────
const AGENT_HUB_URL = process.env.AGENT_HUB_URL || "http://localhost:8081";

// 代理处理器：/api/v1 与 /auth 共用（agent-hub 登录接口在 /auth 前缀下）
const agentHubProxy = (req: express.Request, res: express.Response) => {
  const base = new URL(AGENT_HUB_URL);
  const isHttps = base.protocol === "https:";
  const target = new URL(req.originalUrl, AGENT_HUB_URL);

  const headers: Record<string, string> = {};
  for (const [k, v] of Object.entries(req.headers)) {
    if (typeof v === "string" && k !== "host" && k !== "content-length" && k !== "connection") {
      headers[k] = v;
    }
  }
  headers["host"] = base.host;
  // 线上 Kong 有跨域校验：带外域 Origin/Referer 的 POST/PUT/DELETE 会被 403。
  // 代理属于服务器对服务器转发，剥掉浏览器来源头即可（GET 不受影响，顺带统一处理）。
  delete headers["origin"];
  delete headers["referer"];

  const transport = isHttps ? https : http;
  const proxyReq = transport.request(
    {
      hostname: target.hostname,
      port: target.port || (isHttps ? 443 : 80),
      path: target.pathname + target.search,
      method: req.method,
      headers,
    },
    (proxyRes) => {
      res.status(proxyRes.statusCode || 502);
      for (const [k, v] of Object.entries(proxyRes.headers)) {
        if (typeof v === "string" && k !== "transfer-encoding" && k !== "connection") {
          res.setHeader(k, v);
        }
      }
      proxyRes.pipe(res);
    }
  );
  proxyReq.on("error", (err) => {
    console.error("[agent-hub proxy]", err.message);
    if (!res.headersSent) {
      res.status(502).json({ success: false, error: `agent-hub 后端不可达（${AGENT_HUB_URL}）` });
    } else {
      res.end();
    }
  });

  // express.json 已消费过 JSON body 的情况：重新序列化转发；其余（multipart 等）直接管道透传
  if (req.body && typeof req.body === "object" && Object.keys(req.body).length > 0) {
    const payload = JSON.stringify(req.body);
    proxyReq.setHeader("content-type", "application/json");
    proxyReq.setHeader("content-length", Buffer.byteLength(payload));
    proxyReq.end(payload);
  } else {
    req.pipe(proxyReq);
  }
};

app.use("/api/v1", agentHubProxy);
app.use("/auth", agentHubProxy);

// Vite middleware setup
async function startServer() {
  if (process.env.NODE_ENV !== "production") {
    const { createServer: createViteServer } = await import("vite");
    const vite = await createViteServer({
      server: { middlewareMode: true },
      appType: "spa",
    });
    app.use(vite.middlewares);
  } else {
    const distPath = path.join(process.cwd(), "dist");
    app.use(express.static(distPath));
    app.get("*", (_req, res) => {
      res.sendFile(path.join(distPath, "index.html"));
    });
  }

  app.listen(PORT, "0.0.0.0", () => {
    console.log(`WorkBuddy server running on http://0.0.0.0:${PORT}`);
  });
}

startServer();
