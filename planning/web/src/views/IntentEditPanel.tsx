/**
 * 「编辑意图」面板 —— 把 `IntentEditor` 接到项目版本库上。
 *
 * ## 职责边界
 *
 * `IntentEditor` 只做「六段表格 + 改一格」（纯展示 + 调 API）；
 * 这里负责三件它不该管的事：
 *
 * 1. **载入哪份文档** —— 指定了项目就读它，否则读库里最近更新的那个；
 * 2. **草稿与版本的关系** —— 编辑过程中的改动只在本组件 state 里，
 *    点「保存为新版本」才落库。这样「改十次存一次」；
 * 3. **保存后刷新生成结果** —— 保存成功后把「写进了哪个项目的哪一版」
 *    原样交给父组件，让它立刻重新生成。
 *
 * ## 为什么入口在这里而不是「项目」页
 *
 * 用户的动作顺序是「改意图 → 看生成结果变了没」。把编辑放在设计页，
 * 改完保存就能直接往下滚看到新结果；放在项目页则要来回切两页。
 * 项目页管的是「历史版本与差异」，那是另一个动作（回溯、比对、交档）。
 */

import { useCallback, useEffect, useState } from 'react';
import type { JSX } from 'react';

import type { IntentCheck, IntentRowResult } from '../api/client';
import { api } from '../api/client';
import { IntentEditor } from './IntentEditor';

export interface SavedRef {
  project: string;
  version: number;
}

export interface IntentEditPanelProps {
  /**
   * 保存成功后把「存进哪个项目的哪一版」交给父组件。
   *
   * ## 为什么要把 project/version 一起传，而不是让父组件读 state
   *
   * `onSourceChange` 与 `onSaved` 在同一个事件处理里先后调用。前者
   * setState 之后，**当前这次渲染的闭包**里父组件的 `editorSource` 仍是旧值。
   * 父组件若靠它判断该从哪读，会退化成「又读了一遍磁盘文件」——
   * 表现就是「保存成功但布线表没变」原样复现。
   */
  onSaved: (saved: SavedRef) => void;
  /**
   * 当前编辑的项目名 + 版本号一旦确定就上报。
   *
   * ## 为什么必须上报
   *
   * 生成走 `GET /api/design`，不带 `project` 时按「库里有项目就读库」，
   * 库里可能有多个项目。父组件得知道现在改的是哪一个，才能保证
   * 「改完保存后重新生成读的正是它」，而不是恰好排在列表第一个的那个。
   */
  onSourceChange: (source: SavedRef | null) => void;
  /** 指定编辑哪个项目；空 = 库里最近更新的那个。 */
  projectName?: string;
}

export function IntentEditPanel({
  onSaved,
  onSourceChange,
  projectName = '',
}: IntentEditPanelProps): JSX.Element {
  const [name, setName] = useState<string | null>(null);
  const [doc, setDoc] = useState<Record<string, unknown> | null>(null);
  const [check, setCheck] = useState<IntentCheck | null>(null);
  const [version, setVersion] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setErr(null);
    try {
      const list = await api.projects();
      // 指定了项目就用它；否则用列表第一个。
      //
      // 后端的 `/api/projects` 按 `updated_at DESC` 排，第一个就是最近改过的。
      // 这里不重复实现「挑哪个」的逻辑：真正决定默认值的是后端
      // （`default_project_name`），前端只负责和它保持一致。
      const chosen = projectName
        ? list.projects.find((p) => p.name === projectName)
        : list.projects[0];
      if (!chosen) {
        setErr(
          projectName
            ? `项目「${projectName}」不在库里。请先在「项目与版本」页保存一次意图。`
            : '还没有任何项目。请先在「项目与版本」页建一个项目并保存一次意图，' +
              '之后就能在这里改格子。',
        );
        setDoc(null);
        onSourceChange(null);
        return;
      }
      const payload = await api.projectIntent(chosen.name);
      const loaded = payload.doc as Record<string, unknown>;
      setName(chosen.name);
      setVersion(payload.version);
      setDoc(loaded);
      onSourceChange({ project: chosen.name, version: payload.version });
      setCheck(await api.intentCheck(loaded));
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [onSourceChange, projectName]);

  useEffect(() => {
    void load();
  }, [load]);

  const onChanged = useCallback((result: IntentRowResult) => {
    setDoc(result.doc);
    setCheck(result.check);
    setMsg(null);
  }, []);

  const onSave = useCallback(
    async (note: string) => {
      if (!name || !doc) return;
      setSaving(true);
      setErr(null);
      setMsg(null);
      try {
        // 先做结构校验（能不能真的转成 Intent），再落库。
        // 只跑业务校验不够：字段缺失时 intent_from_dict 会抛，落库阶段才炸就晚了。
        const validate = await api.intentValidate(doc);
        if (!validate.generatable) {
          setErr(
            validate.structuralError
              ? `结构不合法，保存会失败：${validate.structuralError}`
              : `还有 ${validate.errorCount} 个错误未修正`,
          );
          return;
        }
        const saved = await api.saveProjectIntent(name, doc, note);
        setVersion(saved.version);
        const ref: SavedRef = { project: name, version: saved.version };
        onSourceChange(ref);
        setMsg(
          saved.isNew
            ? `已保存为 v${saved.version}（${saved.summary.total} 行意图）`
            : `内容没有变化，仍是 v${saved.version}`,
        );
        onSaved(ref);
      } catch (e) {
        setErr(e instanceof Error ? e.message : String(e));
      } finally {
        setSaving(false);
      }
    },
    [doc, name, onSaved, onSourceChange],
  );

  if (loading) {
    return <div className="view-placeholder">正在读取意图…</div>;
  }

  if (err && !doc) {
    return (
      <section className="panel">
        <div className="error-box">{err}</div>
      </section>
    );
  }

  if (!doc) return <></>;

  return (
    <div className="intent-edit-panel">
      <div className="edit-meta">
        <span>
          正在编辑项目 <b>{name}</b> 的最新版本 <b>v{version ?? '?'}</b>
        </span>
        <button className="btn-secondary btn-tiny" onClick={() => void load()} disabled={saving}>
          放弃改动，重新载入
        </button>
      </div>
      {msg && <div className="ok-box">{msg}</div>}
      {err && <div className="error-box">{err}</div>}
      <IntentEditor
        doc={doc}
        check={check}
        onChanged={onChanged}
        onSave={onSave}
        saving={saving}
      />
    </div>
  );
}
