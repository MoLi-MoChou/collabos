import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError, useApi, useCore } from "@collabos/core";
import { Button, Card, Input, Label } from "@collabos/ui";

/** Dev login (COLLAB_DEV_LOGIN). With an invite code the user joins that workspace. */
export function LoginView({ needsPassword = false }: { needsPassword?: boolean }) {
  const { t } = useTranslation();
  const api = useApi();
  const { store } = useCore();
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [handle, setHandle] = useState("");
  const [invite, setInvite] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      const r = await api.devLogin({ email, name, handle, invite_code: invite, password });
      store.getState().setSession({ token: r.token, userId: r.user_id, name: r.name, handle: r.handle, workspaceId: r.workspace_id });
    } catch (x) {
      setErr(x instanceof ApiError ? x.message : t("common.error"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex h-full items-center justify-center bg-bg p-6">
      <Card className="w-full max-w-sm p-6">
        <div className="mb-5">
          <div className="text-heading font-semibold">{t("login.title")}</div>
          <div className="mt-1 text-body text-fg-muted">{t("login.subtitle")}</div>
        </div>
        <form className="flex flex-col gap-3" onSubmit={submit} data-testid="login-form">
          <div className="flex flex-col gap-1">
            <Label htmlFor="email">{t("login.email")}</Label>
            <Input id="email" name="email" type="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
          </div>
          <div className="flex gap-2">
            <div className="flex flex-1 flex-col gap-1">
              <Label htmlFor="name">{t("login.name")}</Label>
              <Input id="name" name="name" value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <div className="flex flex-1 flex-col gap-1">
              <Label htmlFor="handle">{t("login.handle")}</Label>
              <Input id="handle" name="handle" value={handle} onChange={(e) => setHandle(e.target.value)} />
            </div>
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor="invite">{t("login.invite")}</Label>
            <Input id="invite" name="invite" value={invite} onChange={(e) => setInvite(e.target.value)} placeholder={t("login.invitePlaceholder")} />
          </div>
          {needsPassword && (
            <div className="flex flex-col gap-1">
              <Label htmlFor="password">{t("login.password")}</Label>
              <Input id="password" name="password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
            </div>
          )}
          {err && <div className="text-body text-danger" data-testid="login-error">{err}</div>}
          <Button variant="primary" type="submit" disabled={busy} className="mt-1 justify-center">
            {t("login.submit")}
          </Button>
        </form>
      </Card>
    </div>
  );
}
