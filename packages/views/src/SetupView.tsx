import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError, useCreateProject } from "@collabos/core";
import { Button, Card, Input, Label } from "@collabos/ui";

/** Shown when the workspace has no project yet. */
export function SetupView() {
  const { t } = useTranslation();
  const create = useCreateProject();
  const [name, setName] = useState("");
  const [remote, setRemote] = useState("");
  const [branch, setBranch] = useState("main");
  return (
    <div className="flex h-full items-center justify-center p-6">
      <Card className="w-full max-w-md p-6">
        <div className="text-heading font-semibold">{t("setup.title")}</div>
        <div className="mt-1 mb-4 text-body text-fg-muted">{t("setup.subtitle")}</div>
        <form
          className="flex flex-col gap-3"
          data-testid="project-form"
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate({ name, remote_url: remote, default_branch: branch });
          }}
        >
          <div className="flex flex-col gap-1">
            <Label htmlFor="pname">{t("setup.name")}</Label>
            <Input id="pname" required value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor="premote">{t("setup.remote")}</Label>
            <Input id="premote" required value={remote} onChange={(e) => setRemote(e.target.value)} placeholder={t("setup.remotePlaceholder")} />
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor="pbranch">{t("setup.branch")}</Label>
            <Input id="pbranch" value={branch} onChange={(e) => setBranch(e.target.value)} />
          </div>
          {create.error && <div className="text-body text-danger">{create.error instanceof ApiError ? create.error.message : t("common.error")}</div>}
          <Button variant="primary" type="submit" disabled={create.isPending} className="justify-center">
            {t("setup.create")}
          </Button>
        </form>
      </Card>
    </div>
  );
}
