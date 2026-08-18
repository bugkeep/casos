import React, {useEffect, useState} from "react";
import {Lock, User} from "lucide-react";
import i18next from "i18next";
import * as AccountBackend from "@/backend/AccountBackend";
import * as Setting from "@/Setting";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {Label} from "@/components/ui/label";
import {PasswordInput} from "@/components/shared/password-input";
import {Loading} from "@/components/shared/loading";
import {ResultScreen} from "@/components/shared/misc";

/**
 * Password sign-in. When the server reports that Casdoor is configured this page
 * never renders — it redirects straight into the Casdoor flow instead, which is
 * why the options request happens before anything is shown.
 */
function SigninPage({logo, themeAlgorithm, site}) {
  const [loading, setLoading] = useState(true);
  const [showSignin, setShowSignin] = useState(false);
  const [errorMessage, setErrorMessage] = useState("");
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    AccountBackend.getSigninOptions()
      .then((res) => {
        if (cancelled) {
          return;
        }
        if (res.status === "ok" && res.data?.casdoorAvailable) {
          Setting.initCasdoorSdk(res.data.authConfig);
          window.location.replace(Setting.getSigninUrl());
          return;
        }
        setLoading(false);
        setShowSignin(res.status === "ok" && !res.data?.casdoorAvailable && Boolean(res.data?.signinAvailable));
        setErrorMessage(res.status === "ok" ? "" : res.msg);
        if (res.status === "ok" && res.data?.autoSignin === true) {
          setPassword("123");
        }
      })
      .catch((error) => {
        if (cancelled) {
          return;
        }
        setLoading(false);
        setShowSignin(false);
        setErrorMessage(error.message);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  function handleSubmit(event) {
    event.preventDefault();
    setSubmitting(true);
    AccountBackend.signinWithPassword(username, password)
      .then((res) => {
        if (res.status === "ok") {
          const from = sessionStorage.getItem("from") || "/";
          sessionStorage.removeItem("from");
          Setting.goToLink(from);
          return;
        }
        Setting.showMessage("error", res.msg);
      })
      .catch((error) => Setting.showMessage("error", error.message))
      .finally(() => setSubmitting(false));
  }

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loading type="page" tip={i18next.t("account:Signing in...")} />
      </div>
    );
  }

  if (!showSignin) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <ResultScreen
          status="!"
          title={i18next.t("account:Sign in is unavailable")}
          subTitle={errorMessage || i18next.t("account:Sign in is unavailable - Tooltip")}
        />
      </div>
    );
  }

  const signinLogo = logo || Setting.getLogo(themeAlgorithm || [], site?.logoUrl);

  return (
    <div className="bg-muted/30 flex min-h-screen items-center justify-center p-6">
      <div className="w-full max-w-sm">
        <div className="mb-9 flex justify-center">
          <img src={signinLogo} alt="CasOS" className="h-12 w-auto max-w-[260px] object-contain" />
        </div>

        <form onSubmit={handleSubmit} className="bg-card grid gap-4 rounded-xl border p-6 shadow-sm">
          <div className="grid gap-2">
            <Label htmlFor="username">{i18next.t("general:Username")}</Label>
            <div className="relative">
              <User className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
              <Input
                id="username"
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                placeholder={i18next.t("general:Username")}
                autoComplete="username"
                className="pl-9"
                required
              />
            </div>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="password">{i18next.t("general:Password")}</Label>
            <div className="relative">
              <Lock className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
              <PasswordInput
                id="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder={i18next.t("general:Password")}
                autoComplete="current-password"
                autoFocus
                className="pl-9"
                required
              />
            </div>
          </div>

          <Button type="submit" className="mt-2 w-full" loading={submitting}>
            {i18next.t("account:Sign In")}
          </Button>
        </form>
      </div>
    </div>
  );
}

export default SigninPage;
