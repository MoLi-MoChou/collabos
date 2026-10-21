import { useTranslation } from "react-i18next";
import type { DockTab } from "@collabos/core";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@collabos/ui";
import { useUi } from "../hooks";
import { TasksTab } from "./TasksTab";
import { PeopleTab } from "./PeopleTab";
import { MemoryTab } from "./MemoryTab";

function isTab(v: string): v is DockTab {
  return v === "tasks" || v === "people" || v === "memory";
}

export function Dock({ roomId }: { roomId: string }) {
  const { t } = useTranslation();
  const ui = useUi();
  return (
    <Tabs value={ui.dockTab} onValueChange={(v) => isTab(v) && ui.actions.setDockTab(v)} className="flex h-full min-h-0 flex-col">
      <TabsList>
        <TabsTrigger value="tasks" data-testid="tab-tasks">{t("dock.tasks")}</TabsTrigger>
        <TabsTrigger value="people" data-testid="tab-people">{t("dock.people")}</TabsTrigger>
        <TabsTrigger value="memory" data-testid="tab-memory">{t("dock.memory")}</TabsTrigger>
      </TabsList>
      <TabsContent value="tasks">
        <TasksTab />
      </TabsContent>
      <TabsContent value="people">
        <PeopleTab />
      </TabsContent>
      <TabsContent value="memory">
        <MemoryTab roomId={roomId} />
      </TabsContent>
    </Tabs>
  );
}
