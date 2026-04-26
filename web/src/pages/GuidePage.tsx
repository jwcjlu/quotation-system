import { useEffect, useState } from 'react'
import stepLoginImage from '../assets/guide/step-login.png'
import stepNewBomImage from '../assets/guide/step-new-bom.png'
import stepBomSearchStatusImage from '../assets/guide/step-bom-search-status.png'
import stepBomMatchImage from '../assets/guide/step-bom-match.png'
import stepMatchDetailImage from '../assets/guide/step-match-detail.png'
import sessionDashboardImage from '../assets/guide/session-dashboard.png'

type PreviewImage = {
  src: string
  alt: string
  title: string
}

const guideSteps = [
  {
    title: '1. 登录系统',
    summary: '进入系统后确认当前账号、角色和可用导航。管理员会看到脚本包、Agent 运维和 HS 元数据等管理入口。',
    image: stepLoginImage,
    alt: '登录后的系统首页',
  },
  {
    title: '2. 新建 BOM 单',
    summary: '在 BOM 会话中点击新建 BOM 单，填写会话信息，选择平台，上传 Excel，并配置解析模式和列映射。',
    image: stepNewBomImage,
    alt: '新建 BOM 单弹窗',
  },
  {
    title: '3. 查看 BOM 搜索状态',
    summary: '上传后进入会话看板，确认导入进度、搜索状态、平台勾选和 BOM 行数据，等待状态变为 data_ready。',
    image: stepBomSearchStatusImage,
    alt: 'BOM 搜索状态页面',
  },
  {
    title: '4. 进入 BOM 配单',
    summary: '状态就绪后点击配单，查看自动推荐结果、排序方式、匹配状态、合计金额和各行候选结果。',
    image: stepBomMatchImage,
    alt: 'BOM 配单结果页面',
  },
  {
    title: '5. 查看配单详情',
    summary: '展开或调整字段后查看更完整的配单明细，包括库存、单价、HS 编码、商检和税率等信息。',
    image: stepMatchDetailImage,
    alt: '配单详情字段页面',
  },
  {
    title: '6. 维护会话明细',
    summary: '需要补充或修正数据时，可回到会话看板维护单据信息、平台勾选和 BOM 行。',
    image: sessionDashboardImage,
    alt: '完整会话看板页面',
  },
] as const

const featureEntries = [
  ['BOM 会话', '创建、上传、查看导入进度，并维护 BOM 行。'],
  ['配单结果', '按价格、库存、货期或综合策略查看推荐结果。'],
  ['配单详情', '查看 HS 编码、税率、商检、库存和金额等详细字段。'],
  ['HS 型号解析', '对需要归类的型号进行 HS 编码候选解析。'],
  ['HS 元数据', '维护分类元数据、HS 条目和同步任务。'],
  ['Agent 运维', '查看 Agent 状态、脚本包和任务执行情况。'],
] as const

function PreviewButton(props: PreviewImage) {
  const { src, alt, title } = props
  return (
    <button
      type="button"
      className="group relative block h-72 w-full overflow-hidden bg-slate-100 text-left"
      onClick={() => window.dispatchEvent(new CustomEvent<PreviewImage>('guide:image-preview', { detail: { src, alt, title } }))}
    >
      <img src={src} alt={alt} className="h-full w-full object-contain p-2 transition duration-200 group-hover:scale-[1.015]" />
      <span className="absolute right-3 top-3 rounded-md bg-slate-950/75 px-2 py-1 text-xs font-medium text-white opacity-0 shadow-sm transition group-hover:opacity-100">
        点击放大
      </span>
    </button>
  )
}

export function GuidePage() {
  const [preview, setPreview] = useState<PreviewImage | null>(null)

  useEffect(() => {
    const handlePreview = (event: Event) => setPreview((event as CustomEvent<PreviewImage>).detail)
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setPreview(null)
    }
    window.addEventListener('guide:image-preview', handlePreview)
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      window.removeEventListener('guide:image-preview', handlePreview)
      window.removeEventListener('keydown', handleKeyDown)
    }
  }, [])

  return (
    <div className="space-y-6">
      <section className="grid gap-6 rounded-lg border border-slate-200 bg-white p-6 shadow-sm lg:grid-cols-[minmax(0,1fr)_22rem]">
        <div>
          <p className="text-xs font-semibold uppercase text-blue-600">WORKFLOW</p>
          <h2 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950">使用指南</h2>
          <p className="mt-3 max-w-3xl text-sm leading-7 text-slate-600">
            这份指南按真实操作截图整理，从登录、新建 BOM、查看搜索状态，到进入配单和查看配单详情。所有截图都可以点击放大查看。
          </p>
        </div>
        <div className="rounded-lg border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-600">
          <div className="font-medium text-slate-900">主要流程</div>
          <div className="mt-2 grid gap-2">
            <span>登录系统</span>
            <span>新建 BOM 单</span>
            <span>BOM 搜索状态</span>
            <span>BOM 配单</span>
            <span>配单详情</span>
          </div>
        </div>
      </section>

      <section className="rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
        <h3 className="text-lg font-semibold text-slate-950">功能范围</h3>
        <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {featureEntries.map(([title, body]) => (
            <div key={title} className="rounded-lg border border-slate-200 bg-slate-50 p-3">
              <div className="text-sm font-medium text-slate-950">{title}</div>
              <p className="mt-1 text-xs leading-5 text-slate-600">{body}</p>
            </div>
          ))}
        </div>
      </section>

      <section className="grid gap-5 lg:grid-cols-2">
        {guideSteps.map((step) => (
          <article key={step.title} className="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
            <PreviewButton src={step.image} alt={step.alt} title={step.title} />
            <div className="p-4">
              <h3 className="text-base font-semibold text-slate-950">{step.title}</h3>
              <p className="mt-2 text-sm leading-6 text-slate-600">{step.summary}</p>
            </div>
          </article>
        ))}
      </section>

      {preview && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/80 p-4" role="dialog" aria-modal="true" aria-label={`${preview.title} 大图预览`}>
          <div className="flex max-h-full w-full max-w-6xl flex-col overflow-hidden rounded-lg bg-white shadow-2xl">
            <div className="flex items-center justify-between gap-3 border-b border-slate-200 px-4 py-3">
              <div className="min-w-0 truncate text-sm font-semibold text-slate-950">{preview.title}</div>
              <button type="button" className="rounded-md border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-50" onClick={() => setPreview(null)}>
                关闭
              </button>
            </div>
            <div className="overflow-auto bg-slate-100 p-3">
              <img src={preview.src} alt={preview.alt} className="mx-auto max-h-[80vh] max-w-none rounded-md border border-slate-200 bg-white object-contain" />
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
